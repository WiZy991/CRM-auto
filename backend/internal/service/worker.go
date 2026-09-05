package service

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/autoimport/crm/internal/store"
)

// Worker — фоновые периодические задачи.
//
// Отдельного планировщика или очереди здесь нет намеренно: все задачи
// идемпотентны, выполняются раз в несколько минут и не требуют гарантий
// доставки. Внешний брокер ради переноса счётчиков и очистки старых строк
// добавил бы ещё одну точку отказа, ничего не улучшив.
type Worker struct {
	banners       *Banners
	comms         *store.DealComms
	notify        *store.Notifications
	sessions      *store.Sessions
	social        *Social
	log           *slog.Logger
	remindedTasks map[string]time.Time
}

func NewWorker(
	banners *Banners,
	comms *store.DealComms,
	notify *store.Notifications,
	sessions *store.Sessions,
	log *slog.Logger,
	social *Social,
) *Worker {
	return &Worker{
		banners:       banners,
		comms:         comms,
		notify:        notify,
		sessions:      sessions,
		social:        social,
		log:           log,
		remindedTasks: make(map[string]time.Time),
	}
}

// Интервалы фоновых задач.
//
// Счётчики сбрасываются часто, чтобы дилер видел свежую статистику;
// уборка старых записей — редко, потому что дорога и не срочна.
const (
	flushInterval   = 2 * time.Minute
	remindInterval  = 10 * time.Minute
	cleanupInterval = 6 * time.Hour
	socialInterval  = 1 * time.Minute
)

// Run запускает фоновые задачи до отмены контекста.
func (w *Worker) Run(ctx context.Context) {
	var wg sync.WaitGroup

	if w.social != nil {
		w.safeRun(ctx, "публикации в каналы дилеров", w.flushSocial)
	}

	tasks := []struct {
		name     string
		interval time.Duration
		run      func(context.Context)
	}{
		{"перенос счётчиков баннеров", flushInterval, w.flushBanners},
		{"публикации в каналы дилеров", socialInterval, w.flushSocial},
		{"напоминания по задачам", remindInterval, w.remindTasks},
		{"очистка устаревших записей", cleanupInterval, w.cleanup},
	}

	for _, task := range tasks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.loop(ctx, task.name, task.interval, task.run)
		}()
	}

	wg.Wait()
	w.log.Info("фоновые задачи остановлены")
}

// loop выполняет задачу по расписанию.
//
// Паника внутри задачи не должна ронять процесс: обработчики запросов
// защищены своим восстановлением, а фоновая горутина без него утащила бы
// за собой весь сервис.
func (w *Worker) loop(ctx context.Context, name string, interval time.Duration, run func(context.Context)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.safeRun(ctx, name, run)
		}
	}
}

func (w *Worker) safeRun(ctx context.Context, name string, run func(context.Context)) {
	defer func() {
		if recovered := recover(); recovered != nil {
			w.log.Error("паника в фоновой задаче",
				slog.String("task", name), slog.Any("panic", recovered))
		}
	}()

	// Собственный таймаут: зависшая задача не должна занимать соединение
	// пула до следующего тика.
	runCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	run(runCtx)
}

func (w *Worker) flushBanners(ctx context.Context) {
	if err := w.banners.FlushCounters(ctx); err != nil {
		w.log.Error("не удалось перенести счётчики баннеров", slog.String("error", err.Error()))
	}

	expired, err := w.banners.ExpireOutdated(ctx)
	if err != nil {
		w.log.Error("не удалось закрыть просроченные баннеры", slog.String("error", err.Error()))
		return
	}
	if expired > 0 {
		w.log.Info("закрыты просроченные баннеры", slog.Int64("count", expired))
	}
}

// remindTasks уведомляет исполнителей о подходящих сроках задач.
//
// Отправленные напоминания запоминаются в памяти процесса: иначе каждый
// прогон слал бы уведомление по одной и той же задаче, и раздел уведомлений
// превратился бы в поток дубликатов.
func (w *Worker) remindTasks(ctx context.Context) {
	tasks, err := w.comms.DueTasks(ctx, 24*time.Hour, 200)
	if err != nil {
		w.log.Error("не удалось выбрать задачи с подходящим сроком", slog.String("error", err.Error()))
		return
	}

	now := time.Now()
	items := make([]store.CreateNotificationParams, 0, len(tasks))

	for _, task := range tasks {
		if task.AssigneeID == nil {
			continue
		}

		key := task.ID.String()
		if sentAt, ok := w.remindedTasks[key]; ok && now.Sub(sentAt) < 24*time.Hour {
			continue
		}
		w.remindedTasks[key] = now

		title := "Приближается срок задачи"
		if task.Overdue(now) {
			title = "Срок задачи прошёл"
		}

		items = append(items, store.CreateNotificationParams{
			UserID: *task.AssigneeID,
			Kind:   store.NotifyDealTaskDue,
			Title:  title,
			Body:   task.Title,
			Link:   "/dealer/deals/" + task.DealID.String(),
		})
	}

	if len(items) == 0 {
		return
	}
	if err := w.notify.CreateMany(ctx, items); err != nil {
		w.log.Error("не удалось разослать напоминания", slog.String("error", err.Error()))
	}
}

func (w *Worker) flushSocial(ctx context.Context) {
	if w.social == nil {
		return
	}
	w.social.ProcessOutbox(ctx)
}

func (w *Worker) cleanup(ctx context.Context) {
	w.forgetOldReminders()

	if removed, err := w.sessions.DeleteExpired(ctx, 30*24*time.Hour); err != nil {
		w.log.Error("не удалось очистить сессии", slog.String("error", err.Error()))
	} else if removed > 0 {
		w.log.Info("удалены устаревшие сессии", slog.Int64("count", removed))
	}

	if removed, err := w.notify.DeleteOld(ctx, 90*24*time.Hour); err != nil {
		w.log.Error("не удалось очистить уведомления", slog.String("error", err.Error()))
	} else if removed > 0 {
		w.log.Info("удалены прочитанные уведомления", slog.Int64("count", removed))
	}
}

// forgetOldReminders убирает из памяти отметки о старых напоминаниях.
//
// Без этого карта растёт весь срок жизни процесса: задачи создаются
// постоянно, а удалённые из базы никогда бы из неё не исчезли.
func (w *Worker) forgetOldReminders() {
	threshold := time.Now().Add(-48 * time.Hour)
	for key, sentAt := range w.remindedTasks {
		if sentAt.Before(threshold) {
			delete(w.remindedTasks, key)
		}
	}
}
