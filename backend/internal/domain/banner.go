package domain

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

// BannerPlacement — место показа баннера.
type BannerPlacement string

const (
	PlacementHomeHero       BannerPlacement = "home_hero"
	PlacementHomeInline     BannerPlacement = "home_inline"
	PlacementCatalogTop     BannerPlacement = "catalog_top"
	PlacementCatalogSidebar BannerPlacement = "catalog_sidebar"
	PlacementCarPage        BannerPlacement = "car_page"
)

var placementTitles = map[BannerPlacement]string{
	PlacementHomeHero:       "Главная, первый экран",
	PlacementHomeInline:     "Главная, в ленте",
	PlacementCatalogTop:     "Каталог, верх страницы",
	PlacementCatalogSidebar: "Каталог, боковая колонка",
	PlacementCarPage:        "Карточка автомобиля",
}

// PlacementOrder задаёт порядок в справочнике — от самых заметных мест.
var PlacementOrder = []BannerPlacement{
	PlacementHomeHero, PlacementHomeInline,
	PlacementCatalogTop, PlacementCatalogSidebar, PlacementCarPage,
}

func (p BannerPlacement) Valid() bool { _, ok := placementTitles[p]; return ok }

func (p BannerPlacement) Title() string {
	if title, ok := placementTitles[p]; ok {
		return title
	}
	return string(p)
}

// BannerStatus — состояние баннера.
type BannerStatus string

const (
	BannerDraft      BannerStatus = "draft"
	BannerModeration BannerStatus = "moderation"
	BannerActive     BannerStatus = "active"
	BannerPaused     BannerStatus = "paused"
	BannerRejected   BannerStatus = "rejected"
	BannerExpired    BannerStatus = "expired"
)

var bannerStatusTitles = map[BannerStatus]string{
	BannerDraft:      "Черновик",
	BannerModeration: "На проверке",
	BannerActive:     "Показывается",
	BannerPaused:     "Приостановлен",
	BannerRejected:   "Отклонён",
	BannerExpired:    "Срок истёк",
}

func (s BannerStatus) Valid() bool { _, ok := bannerStatusTitles[s]; return ok }

func (s BannerStatus) Title() string {
	if title, ok := bannerStatusTitles[s]; ok {
		return title
	}
	return string(s)
}

// BannerDictionaries возвращает справочники для интерфейса рекламы.
func BannerDictionaries() map[string][]DictionaryEntry {
	return map[string][]DictionaryEntry{
		"placement": dictionary(placementTitles, PlacementOrder),
		"status": dictionary(bannerStatusTitles, []BannerStatus{
			BannerDraft, BannerModeration, BannerActive,
			BannerPaused, BannerRejected, BannerExpired,
		}),
	}
}

// Banner — рекламный баннер дилера.
type Banner struct {
	ID       uuid.UUID
	DealerID uuid.UUID

	Placement BannerPlacement
	Status    BannerStatus

	Title          string
	Subtitle       string
	ImageURL       string
	ImageMobileURL string
	Href           string
	CTALabel       string

	StartsAt time.Time
	EndsAt   time.Time
	Weight   int

	Impressions int64
	Clicks      int64

	RejectReason string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// IsShowable сообщает, можно ли показывать баннер прямо сейчас.
func (b *Banner) IsShowable(now time.Time) bool {
	return b.Status == BannerActive && !now.Before(b.StartsAt) && now.Before(b.EndsAt)
}

// CTR — доля кликов от показов.
func (b *Banner) CTR() float64 {
	if b.Impressions == 0 {
		return 0
	}
	return float64(b.Clicks) / float64(b.Impressions)
}

// Ограничения периода показа.
//
// Верхняя граница нужна, чтобы баннер не занимал место в выдаче годами:
// договор на рекламу конечен, а забытая карточка с истёкшей акцией портит
// впечатление от площадки сильнее, чем пустое место.
const (
	MinBannerDuration = time.Hour
	MaxBannerDuration = 365 * 24 * time.Hour
	MaxBannerLeadTime = 180 * 24 * time.Hour
)

// ValidateBannerPeriod проверяет период показа.
func ValidateBannerPeriod(startsAt, endsAt, now time.Time) error {
	if !endsAt.After(startsAt) {
		return fmt.Errorf("дата окончания должна быть позже даты начала")
	}
	if endsAt.Before(now) {
		return fmt.Errorf("период показа уже закончился")
	}
	if duration := endsAt.Sub(startsAt); duration < MinBannerDuration {
		return fmt.Errorf("минимальный период показа — один час")
	} else if duration > MaxBannerDuration {
		return fmt.Errorf("максимальный период показа — один год")
	}
	if startsAt.Sub(now) > MaxBannerLeadTime {
		return fmt.Errorf("начало показа не может быть дальше чем через полгода")
	}
	return nil
}

// ValidateBannerHref проверяет ссылку баннера.
//
// Схема ограничена http и https явным списком. Ссылка попадает в атрибут
// href на публичной странице, и javascript: или data: там означали бы
// выполнение чужого кода в браузере посетителя.
func ValidateBannerHref(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("укажите ссылку")
	}
	if len(value) > 2000 {
		return "", fmt.Errorf("ссылка слишком длинная")
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("ссылка указана неверно")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("ссылка должна начинаться с http:// или https://")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("в ссылке не указан адрес сайта")
	}
	return parsed.String(), nil
}
