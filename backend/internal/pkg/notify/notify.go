// Package notify отправляет письма и SMS.
//
// Оба канала описаны интерфейсами с несколькими реализациями. Это нужно не
// ради абстракции как таковой: в разработке коды подтверждения должны
// попадать в лог и в локальный почтовый ящик, в тестах — в память, а в
// production — реальному провайдеру. Без подмены реализации тесты
// аутентификации либо не работают, либо спамят настоящими сообщениями.
package notify

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
	"sync"
	"time"

	"github.com/autoimport/crm/internal/config"
	"github.com/autoimport/crm/internal/pkg/logging"
)

// Mailer отправляет письма.
type Mailer interface {
	Send(ctx context.Context, msg Message) error
}

// Message — письмо.
type Message struct {
	To      string
	Subject string
	// Text — обязательная текстовая версия. HTML необязателен.
	Text string
	HTML string
}

// SMSSender отправляет SMS.
type SMSSender interface {
	Send(ctx context.Context, phone, text string) error
}

// --- Реализация для логов ---------------------------------------------------

// LogMailer печатает письмо в лог вместо отправки.
type LogMailer struct {
	log *slog.Logger
}

func NewLogMailer(log *slog.Logger) *LogMailer { return &LogMailer{log: log} }

func (m *LogMailer) Send(ctx context.Context, msg Message) error {
	// Тема и получатель маскируются логгером, а тело выводится целиком:
	// в разработке именно из него берут код подтверждения.
	m.log.InfoContext(ctx, "письмо (режим лога)",
		slog.String("email", msg.To),
		slog.String("subject", msg.Subject),
		slog.String("body", msg.Text),
	)
	return nil
}

// LogSMS печатает SMS в лог.
type LogSMS struct {
	log *slog.Logger
}

func NewLogSMS(log *slog.Logger) *LogSMS { return &LogSMS{log: log} }

func (s *LogSMS) Send(ctx context.Context, phone, text string) error {
	s.log.InfoContext(ctx, "SMS (режим лога)",
		slog.String("phone", phone),
		slog.String("body", text),
	)
	return nil
}

// --- SMTP -------------------------------------------------------------------

// SMTPMailer отправляет письма через SMTP-сервер.
type SMTPMailer struct {
	host     string
	port     int
	user     string
	password string
	from     string
	log      *slog.Logger
}

func NewSMTPMailer(cfg config.Mailer, log *slog.Logger) *SMTPMailer {
	return &SMTPMailer{
		host: cfg.Host, port: cfg.Port,
		user: cfg.User, password: cfg.Password,
		from: cfg.From, log: log,
	}
}

func (m *SMTPMailer) Send(ctx context.Context, msg Message) error {
	if msg.To == "" {
		return errors.New("не указан получатель письма")
	}

	// Значения, попадающие в заголовки письма, проверяются на перевод
	// строки: иначе через тему или адрес можно внедрить лишние заголовки
	// и превратить сервис в открытый релей для рассылки.
	if hasHeaderInjection(msg.To) || hasHeaderInjection(msg.Subject) {
		return errors.New("недопустимые символы в заголовках письма")
	}

	body := m.compose(msg)
	addr := net.JoinHostPort(m.host, fmt.Sprint(m.port))

	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("подключение к SMTP %s: %w", addr, err)
	}

	client, err := smtp.NewClient(conn, m.host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("инициализация SMTP-клиента: %w", err)
	}
	defer func() { _ = client.Quit() }()

	// STARTTLS применяется, если сервер его поддерживает. Локальный Mailpit
	// работает без шифрования, поэтому отсутствие поддержки не является
	// ошибкой в development.
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: m.host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("установка TLS для SMTP: %w", err)
		}
	}

	if m.user != "" {
		if err := client.Auth(smtp.PlainAuth("", m.user, m.password, m.host)); err != nil {
			return fmt.Errorf("аутентификация на SMTP-сервере: %w", err)
		}
	}

	if err := client.Mail(extractAddress(m.from)); err != nil {
		return fmt.Errorf("команда MAIL FROM: %w", err)
	}
	if err := client.Rcpt(msg.To); err != nil {
		return fmt.Errorf("команда RCPT TO: %w", err)
	}

	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("команда DATA: %w", err)
	}
	if _, err := writer.Write([]byte(body)); err != nil {
		return fmt.Errorf("запись тела письма: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("завершение передачи письма: %w", err)
	}

	m.log.InfoContext(ctx, "письмо отправлено",
		slog.String("email", msg.To),
		slog.String("subject", msg.Subject))
	return nil
}

func (m *SMTPMailer) compose(msg Message) string {
	var b strings.Builder

	fmt.Fprintf(&b, "From: %s\r\n", m.from)
	fmt.Fprintf(&b, "To: %s\r\n", msg.To)
	fmt.Fprintf(&b, "Subject: %s\r\n", encodeHeader(msg.Subject))
	fmt.Fprintf(&b, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\n")

	if msg.HTML == "" {
		b.WriteString("Content-Type: text/plain; charset=\"UTF-8\"\r\n\r\n")
		b.WriteString(msg.Text)
		return b.String()
	}

	boundary := "autoimport-boundary-42"
	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", boundary)
	fmt.Fprintf(&b, "--%s\r\nContent-Type: text/plain; charset=\"UTF-8\"\r\n\r\n%s\r\n", boundary, msg.Text)
	fmt.Fprintf(&b, "--%s\r\nContent-Type: text/html; charset=\"UTF-8\"\r\n\r\n%s\r\n", boundary, msg.HTML)
	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	return b.String()
}

// encodeHeader кодирует тему письма в base64 по RFC 2047: кириллица в
// заголовке иначе отображается как набор символов.
func encodeHeader(value string) string {
	for _, r := range value {
		if r > 127 {
			return mimeEncode(value)
		}
	}
	return value
}

func hasHeaderInjection(value string) bool {
	return strings.ContainsAny(value, "\r\n")
}

func extractAddress(from string) string {
	if start := strings.LastIndex(from, "<"); start >= 0 {
		if end := strings.Index(from[start:], ">"); end > 0 {
			return from[start+1 : start+end]
		}
	}
	return strings.TrimSpace(from)
}

// --- Отправка в памяти (для тестов) -----------------------------------------

// MemoryMailer запоминает письма вместо отправки.
type MemoryMailer struct {
	mu       sync.Mutex
	Messages []Message
}

func NewMemoryMailer() *MemoryMailer { return &MemoryMailer{} }

func (m *MemoryMailer) Send(_ context.Context, msg Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Messages = append(m.Messages, msg)
	return nil
}

// Last возвращает последнее письмо.
func (m *MemoryMailer) Last() (Message, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Messages) == 0 {
		return Message{}, false
	}
	return m.Messages[len(m.Messages)-1], true
}

// MemorySMS запоминает SMS вместо отправки.
type MemorySMS struct {
	mu       sync.Mutex
	Messages []string
	Phones   []string
}

func NewMemorySMS() *MemorySMS { return &MemorySMS{} }

func (s *MemorySMS) Send(_ context.Context, phone, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Phones = append(s.Phones, phone)
	s.Messages = append(s.Messages, text)
	return nil
}

// --- Фабрики ---------------------------------------------------------------

// NewMailer выбирает реализацию по конфигурации.
func NewMailer(cfg config.Mailer, log *slog.Logger) Mailer {
	switch strings.ToLower(cfg.Driver) {
	case "smtp":
		return NewSMTPMailer(cfg, log)
	default:
		return NewLogMailer(log)
	}
}

// NewSMS выбирает реализацию отправки SMS.
//
// Драйвер реального провайдера не реализован: подключать конкретного
// оператора без договора и ключей бессмысленно, а заглушка в логе
// полностью покрывает разработку и тесты. Точка расширения — этот switch.
func NewSMS(cfg config.SMS, log *slog.Logger) SMSSender {
	switch strings.ToLower(cfg.Driver) {
	default:
		return NewLogSMS(log)
	}
}

// MaskedRecipient возвращает получателя в замаскированном виде.
func MaskedRecipient(value string) string { return logging.Mask(value) }
