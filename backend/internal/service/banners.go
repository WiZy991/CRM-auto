package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/autoimport/crm/internal/domain"
	"github.com/autoimport/crm/internal/pkg/apierr"
	"github.com/autoimport/crm/internal/store"
)

// Banners — сценарии работы с рекламными баннерами дилеров.
type Banners struct {
	banners  *store.Banners
	counters *BannerCounters
	notify   *store.Notifications
	audit    *store.SecurityLog
	log      *slog.Logger
}

func NewBanners(
	banners *store.Banners,
	counters *BannerCounters,
	notify *store.Notifications,
	audit *store.SecurityLog,
	log *slog.Logger,
) *Banners {
	return &Banners{banners: banners, counters: counters, notify: notify, audit: audit, log: log}
}

// Ограничения на баннеры одного дилера.
//
// Предел нужен не ради нагрузки, а ради выдачи: без него один дилер
// занимает все места показа, и реклама перестаёт быть площадкой.
const maxBannersPerDealer = 20

// BannerForm — данные баннера из формы.
type BannerForm struct {
	Placement string

	Title          string
	Subtitle       string
	ImageURL       string
	ImageMobileURL string
	Href           string
	CTALabel       string

	StartsAt time.Time
	EndsAt   time.Time
	Weight   int
}

// Create создаёт баннер в состоянии черновика.
func (b *Banners) Create(ctx context.Context, dealerID uuid.UUID, form BannerForm) (*domain.Banner, error) {
	existing, err := b.banners.ListForDealer(ctx, dealerID)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	if len(existing) >= maxBannersPerDealer {
		return nil, apierr.Conflict(fmt.Sprintf(
			"Достигнут предел в %d баннеров. Удалите неиспользуемые.", maxBannersPerDealer))
	}

	params, err := b.buildParams(form)
	if err != nil {
		return nil, err
	}
	params.DealerID = dealerID

	banner, err := b.banners.Create(ctx, *params)
	if err != nil {
		return nil, apierr.Internal(err)
	}

	b.recordAudit(ctx, dealerID, "banner.create", banner.ID.String(),
		map[string]any{"placement": string(banner.Placement)})
	return banner, nil
}

// Update изменяет баннер.
func (b *Banners) Update(ctx context.Context, bannerID uuid.UUID, viewer Viewer, form BannerForm) (*domain.Banner, error) {
	params, err := b.buildParams(form)
	if err != nil {
		return nil, err
	}

	banner, err := b.banners.Update(
		ctx, bannerID, viewer.UserID, viewer.Role == domain.RoleAdmin, *params)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, apierr.NotFound("Баннер")
		}
		return nil, apierr.Internal(err)
	}

	b.recordAudit(ctx, viewer.UserID, "banner.update", bannerID.String(), nil)
	return banner, nil
}

func (b *Banners) buildParams(form BannerForm) (*store.BannerParams, error) {
	placement := domain.BannerPlacement(normalizeEnum(form.Placement))
	if !placement.Valid() {
		return nil, apierr.BadRequest("Неизвестное место показа")
	}

	title := normalizeShortText(form.Title, 120)
	if len([]rune(title)) < 2 {
		return nil, apierr.BadRequest("Заголовок должен быть не короче двух символов")
	}

	href, err := domain.ValidateBannerHref(form.Href)
	if err != nil {
		return nil, apierr.BadRequest(err.Error())
	}

	image := normalizeShortText(form.ImageURL, 500)
	if image == "" {
		return nil, apierr.BadRequest("Загрузите изображение баннера")
	}

	if err := domain.ValidateBannerPeriod(form.StartsAt, form.EndsAt, time.Now()); err != nil {
		return nil, apierr.BadRequest(err.Error())
	}

	weight := form.Weight
	if weight == 0 {
		weight = 10
	}
	if weight < 1 || weight > 100 {
		return nil, apierr.BadRequest("Вес показа должен быть от 1 до 100")
	}

	cta := normalizeShortText(form.CTALabel, 40)
	if cta == "" {
		cta = "Подробнее"
	}

	return &store.BannerParams{
		Placement:      placement,
		Title:          title,
		Subtitle:       normalizeShortText(form.Subtitle, 200),
		ImageURL:       image,
		ImageMobileURL: normalizeShortText(form.ImageMobileURL, 500),
		Href:           href,
		CTALabel:       cta,
		StartsAt:       form.StartsAt,
		EndsAt:         form.EndsAt,
		Weight:         weight,
	}, nil
}

// ListForDealer возвращает баннеры дилера со сводкой.
func (b *Banners) ListForDealer(ctx context.Context, dealerID uuid.UUID) ([]domain.Banner, *store.BannerStats, error) {
	items, err := b.banners.ListForDealer(ctx, dealerID)
	if err != nil {
		return nil, nil, apierr.Internal(err)
	}

	stats, err := b.banners.StatsForDealer(ctx, dealerID)
	if err != nil {
		return nil, nil, apierr.Internal(err)
	}

	// Счётчики из Redis добавляются к сохранённым: иначе дилер видит
	// статистику с задержкой до следующего сброса и считает её сломанной.
	b.counters.Merge(ctx, items)
	return items, stats, nil
}

// Submit отправляет баннер на модерацию.
func (b *Banners) Submit(ctx context.Context, bannerID, dealerID uuid.UUID) (*domain.Banner, error) {
	banner, err := b.banners.SubmitForModeration(ctx, bannerID, dealerID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, apierr.Conflict("Баннер нельзя отправить на проверку в текущем состоянии")
		}
		return nil, apierr.Internal(err)
	}
	return banner, nil
}

// SetPaused приостанавливает или возобновляет показ.
func (b *Banners) SetPaused(ctx context.Context, bannerID, dealerID uuid.UUID, paused bool) (*domain.Banner, error) {
	banner, err := b.banners.SetPaused(ctx, bannerID, dealerID, paused)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, apierr.Conflict("Показ нельзя изменить в текущем состоянии баннера")
		}
		return nil, apierr.Internal(err)
	}
	return banner, nil
}

// Delete удаляет баннер.
func (b *Banners) Delete(ctx context.Context, bannerID uuid.UUID, viewer Viewer) error {
	err := b.banners.Delete(ctx, bannerID, viewer.UserID, viewer.Role == domain.RoleAdmin)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return apierr.NotFound("Баннер")
		}
		return apierr.Internal(err)
	}

	b.recordAudit(ctx, viewer.UserID, "banner.delete", bannerID.String(), nil)
	return nil
}

// PendingModeration возвращает баннеры на проверке.
func (b *Banners) PendingModeration(ctx context.Context, limit int) ([]domain.Banner, error) {
	items, err := b.banners.ListForModeration(ctx, limit)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	return items, nil
}

// Moderate одобряет или отклоняет баннер.
func (b *Banners) Moderate(ctx context.Context, bannerID, adminID uuid.UUID, approve bool, reason string) (*domain.Banner, error) {
	if !approve && normalizeShortText(reason, 500) == "" {
		return nil, apierr.BadRequest("Укажите причину отклонения")
	}

	banner, err := b.banners.Moderate(ctx, bannerID, adminID, approve, reason)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, apierr.Conflict("Баннер не находится на проверке")
		}
		return nil, apierr.Internal(err)
	}

	title := "Баннер одобрен"
	body := banner.Title
	if !approve {
		title = "Баннер отклонён"
		body = reason
	}
	if err := b.notify.Create(ctx, store.CreateNotificationParams{
		UserID: banner.DealerID,
		Kind:   store.NotifyModerationResult,
		Title:  title,
		Body:   body,
		Link:   "/dealer/banners",
	}); err != nil {
		b.log.ErrorContext(ctx, "не удалось уведомить о модерации баннера", "error", err)
	}

	b.recordAudit(ctx, adminID, "banner.moderate", bannerID.String(),
		map[string]any{"approved": approve})
	return banner, nil
}

// Active возвращает баннеры для показа и учитывает показы.
func (b *Banners) Active(ctx context.Context, placement string, limit int) ([]domain.Banner, error) {
	value := domain.BannerPlacement(normalizeEnum(placement))
	if !value.Valid() {
		return nil, apierr.BadRequest("Неизвестное место показа")
	}

	items, err := b.banners.Active(ctx, value, limit)
	if err != nil {
		return nil, apierr.Internal(err)
	}

	b.counters.RecordImpressions(ctx, items)
	return items, nil
}

// RegisterClick учитывает переход по баннеру и возвращает целевую ссылку.
func (b *Banners) RegisterClick(ctx context.Context, bannerID uuid.UUID) (string, error) {
	banner, err := b.banners.ByID(ctx, bannerID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return "", apierr.NotFound("Баннер")
		}
		return "", apierr.Internal(err)
	}

	// Переход засчитывается только по действующему баннеру: иначе счётчик
	// накручивается запросами к давно отключённым записям.
	if !banner.IsShowable(time.Now()) {
		return "", apierr.NotFound("Баннер")
	}

	b.counters.RecordClick(ctx, bannerID)
	return banner.Href, nil
}

// FlushCounters переносит накопленные счётчики в базу.
func (b *Banners) FlushCounters(ctx context.Context) error {
	impressions, clicks, err := b.counters.Drain(ctx)
	if err != nil {
		return err
	}
	return b.banners.FlushCounters(ctx, impressions, clicks)
}

// ExpireOutdated закрывает баннеры с истёкшим сроком.
func (b *Banners) ExpireOutdated(ctx context.Context) (int64, error) {
	return b.banners.ExpireOutdated(ctx)
}

func (b *Banners) recordAudit(ctx context.Context, actorID uuid.UUID, action, entityID string, diff map[string]any) {
	if err := b.audit.RecordAudit(ctx, store.AuditEntry{
		ActorID:  &actorID,
		Action:   action,
		Entity:   "banner",
		EntityID: entityID,
		Diff:     diff,
	}); err != nil {
		b.log.ErrorContext(ctx, "не удалось записать аудит баннера", "error", err, "action", action)
	}
}
