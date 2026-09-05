package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"github.com/autoimport/crm/internal/domain"
	"github.com/autoimport/crm/internal/pkg/apierr"
	"github.com/autoimport/crm/internal/store"
)

// Sellers — сценарии работы со справочником зарубежных поставщиков.
type Sellers struct {
	sellers *store.Sellers
	audit   *store.SecurityLog
	log     *slog.Logger
}

func NewSellers(sellers *store.Sellers, audit *store.SecurityLog, log *slog.Logger) *Sellers {
	return &Sellers{sellers: sellers, audit: audit, log: log}
}

// SellerView — карточка продавца, подготовленная к отдаче наружу.
type SellerView struct {
	Seller *domain.Seller
	// Contacts уже приведены к правам запрашивающего.
	Contacts map[string]string
	CanEdit  bool
	// Link — приватная заметка дилера об этом продавце.
	Link *store.SellerLink
}

// List возвращает страницу справочника продавцов.
func (s *Sellers) List(ctx context.Context, filter store.SellerFilter, viewer Viewer) ([]SellerView, int, error) {
	// Скрытые и неактивные карточки видит только администратор: параметр
	// из запроса на это повлиять не может.
	filter.IncludeInactive = filter.IncludeInactive && viewer.Role == domain.RoleAdmin
	filter.OwnerID = nil

	items, total, err := s.sellers.List(ctx, filter)
	if err != nil {
		return nil, 0, apierr.Internal(err)
	}

	views, err := s.decorate(ctx, items, viewer)
	if err != nil {
		return nil, 0, err
	}
	return views, total, nil
}

// ListMine возвращает карточки, принадлежащие продавцу.
func (s *Sellers) ListMine(ctx context.Context, viewer Viewer) ([]SellerView, error) {
	items, _, err := s.sellers.List(ctx, store.SellerFilter{
		OwnerID:         &viewer.UserID,
		IncludeInactive: true,
		Limit:           100,
	})
	if err != nil {
		return nil, apierr.Internal(err)
	}
	return s.decorate(ctx, items, viewer)
}

// decorate дополняет карточки правами и приватными заметками дилера.
//
// Заметки читаются одним запросом на всю страницу: отдельный запрос на
// карточку превратил бы список из двадцати продавцов в двадцать обращений
// к базе.
func (s *Sellers) decorate(ctx context.Context, items []domain.Seller, viewer Viewer) ([]SellerView, error) {
	isDealer := viewer.Role == domain.RoleDealer || viewer.Role == domain.RoleAdmin

	var links map[uuid.UUID]store.SellerLink
	if viewer.Role == domain.RoleDealer && len(items) > 0 {
		ids := make([]uuid.UUID, 0, len(items))
		for index := range items {
			ids = append(ids, items[index].ID)
		}

		found, err := s.sellers.Links(ctx, viewer.UserID, ids)
		if err != nil {
			return nil, apierr.Internal(err)
		}
		links = found
	}

	views := make([]SellerView, 0, len(items))
	for index := range items {
		seller := items[index]

		view := SellerView{
			Seller:   &seller,
			Contacts: seller.PublicContacts(isDealer),
			CanEdit:  viewer.Role == domain.RoleAdmin || seller.OwnedBy(viewer.UserID),
		}
		if link, ok := links[seller.ID]; ok {
			view.Link = &link
		}
		views = append(views, view)
	}
	return views, nil
}

// Get возвращает карточку продавца.
func (s *Sellers) Get(ctx context.Context, sellerID uuid.UUID, viewer Viewer) (*SellerView, error) {
	seller, err := s.sellers.ByID(ctx, sellerID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, apierr.NotFound("Продавец")
		}
		return nil, apierr.Internal(err)
	}

	canEdit := viewer.Role == domain.RoleAdmin || seller.OwnedBy(viewer.UserID)
	if !seller.IsActive && !canEdit {
		return nil, apierr.NotFound("Продавец")
	}

	views, err := s.decorate(ctx, []domain.Seller{*seller}, viewer)
	if err != nil {
		return nil, err
	}
	return &views[0], nil
}

// SellerForm — данные карточки продавца из формы.
type SellerForm struct {
	Country string
	Kind    string

	Name      string
	NameLocal string
	Region    string
	City      string
	Address   string

	Brands      []string
	Description string
	Website     string
	Contacts    map[string]string
	LogoURL     string

	MinOrderQty           int
	ExportExperienceYears *int
}

// Ограничения на карточку продавца.
const (
	maxSellerBrands      = 60
	maxSellerContacts    = 12
	maxContactKeyLen     = 40
	maxContactValueLen   = 200
	maxSellerDescription = 5000
)

// Create добавляет продавца в справочник.
//
// Карточку заводят и сам продавец, и дилер, и администратор. Автор
// фиксируется в created_by, а владельцем становится только пользователь с
// ролью продавца: иначе дилер, заполнивший карточку конкурента, получил бы
// право её редактировать.
func (s *Sellers) Create(ctx context.Context, viewer Viewer, form SellerForm) (*domain.Seller, error) {
	params, err := s.buildParams(form)
	if err != nil {
		return nil, err
	}
	params.CreatedBy = viewer.UserID

	if viewer.Role == domain.RoleSeller {
		params.UserID = &viewer.UserID

		// У продавца может быть только одна карточка: проверка есть и в
		// базе через уникальный индекс, но понятная ошибка лучше отказа
		// по нарушению ограничения.
		if _, err := s.sellers.ByUserID(ctx, viewer.UserID); err == nil {
			return nil, apierr.Conflict("У вас уже есть карточка продавца")
		} else if !errors.Is(err, store.ErrNotFound) {
			return nil, apierr.Internal(err)
		}
	}

	seller, err := s.sellers.Create(ctx, *params)
	if err != nil {
		if errors.Is(err, store.ErrSellerProfileExists) {
			return nil, apierr.Conflict("У вас уже есть карточка продавца")
		}
		return nil, apierr.Internal(err)
	}

	s.recordAudit(ctx, viewer, "seller.create", seller.ID.String(),
		map[string]any{"name": seller.Name, "country": string(seller.Country)})
	return seller, nil
}

// Update изменяет карточку продавца.
func (s *Sellers) Update(ctx context.Context, sellerID uuid.UUID, viewer Viewer, form SellerForm) (*domain.Seller, error) {
	params, err := s.buildParams(form)
	if err != nil {
		return nil, err
	}

	seller, err := s.sellers.Update(
		ctx, sellerID, viewer.UserID, viewer.Role == domain.RoleAdmin, *params)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// Ответ одинаков и для несуществующей карточки, и для чужой:
			// иначе по коду ответа можно перебором выяснить, какие
			// идентификаторы существуют.
			return nil, apierr.NotFound("Продавец")
		}
		return nil, apierr.Internal(err)
	}

	s.recordAudit(ctx, viewer, "seller.update", sellerID.String(), nil)
	return seller, nil
}

func (s *Sellers) buildParams(form SellerForm) (*store.SellerParams, error) {
	country := domain.Origin(normalizeEnum(form.Country))
	if !country.Valid() {
		return nil, apierr.BadRequest("Укажите страну: Китай или Япония")
	}

	kind := domain.SellerKind(normalizeEnum(form.Kind))
	if !kind.Valid() {
		return nil, apierr.BadRequest("Неизвестный тип продавца")
	}

	name := normalizeShortText(form.Name, 200)
	if len([]rune(name)) < 2 {
		return nil, apierr.BadRequest("Название должно быть не короче двух символов")
	}

	region := normalizeShortText(form.Region, 120)
	if region == "" {
		return nil, apierr.BadRequest("Укажите регион или префектуру")
	}

	brands, err := normalizeBrands(form.Brands)
	if err != nil {
		return nil, err
	}

	contacts, err := normalizeContacts(form.Contacts)
	if err != nil {
		return nil, err
	}
	if country == domain.OriginChina && contactValue(contacts, "wechat") == "" {
		return nil, apierr.Validation(map[string]string{
			"wechat": "для поставщика из Китая укажите WeChat",
		})
	}

	website := ""
	if strings.TrimSpace(form.Website) != "" {
		validated, err := validateHTTPURL(form.Website)
		if err != nil {
			return nil, apierr.BadRequest("Адрес сайта: " + err.Error())
		}
		website = validated
	}

	minOrder := form.MinOrderQty
	if minOrder < 1 {
		minOrder = 1
	}
	if minOrder > 1000 {
		return nil, apierr.BadRequest("Минимальная партия указана слишком большой")
	}

	if form.ExportExperienceYears != nil {
		years := *form.ExportExperienceYears
		if years < 0 || years > 100 {
			return nil, apierr.BadRequest("Опыт экспорта указан вне разумных границ")
		}
	}

	return &store.SellerParams{
		Country:   country,
		Kind:      kind,
		Name:      name,
		NameLocal: normalizeShortText(form.NameLocal, 200),
		Region:    region,
		City:      normalizeShortText(form.City, 120),
		Address:   normalizeShortText(form.Address, 300),

		Brands:      brands,
		Description: normalizeShortText(form.Description, maxSellerDescription),
		Website:     website,
		Contacts:    contacts,
		LogoURL:     normalizeShortText(form.LogoURL, 500),

		MinOrderQty:           minOrder,
		ExportExperienceYears: form.ExportExperienceYears,
	}, nil
}

// normalizeBrands приводит список марок к каноничному виду.
//
// Дубликаты убираются с учётом регистра: «Toyota» и «toyota» — одна марка,
// и без свёртки фильтр по марке начинает показывать одного продавца дважды.
func normalizeBrands(values []string) ([]string, error) {
	if len(values) > maxSellerBrands {
		return nil, apierr.BadRequest(fmt.Sprintf("Не больше %d марок в карточке", maxSellerBrands))
	}

	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))

	for _, raw := range values {
		brand := normalizeShortText(raw, 60)
		if brand == "" {
			continue
		}

		key := strings.ToLower(brand)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, brand)
	}
	return out, nil
}

// normalizeContacts проверяет произвольный набор контактов.
//
// Ключи и значения ограничены по длине и количеству: поле свободной формы
// в jsonb иначе превращается в место для хранения килобайтов чужих данных.
func normalizeContacts(values map[string]string) (map[string]string, error) {
	if len(values) > maxSellerContacts {
		return nil, apierr.BadRequest(fmt.Sprintf("Не больше %d контактов в карточке", maxSellerContacts))
	}

	out := make(map[string]string, len(values))
	for key, value := range values {
		cleanKey := normalizeShortText(key, maxContactKeyLen)
		cleanValue := normalizeShortText(value, maxContactValueLen)
		if cleanKey == "" || cleanValue == "" {
			continue
		}
		if strings.EqualFold(cleanKey, "wechat") {
			cleanKey = "wechat"
		}
		out[cleanKey] = cleanValue
	}
	return out, nil
}

func contactValue(contacts map[string]string, names ...string) string {
	for key, value := range contacts {
		for _, name := range names {
			if strings.EqualFold(key, name) {
				return value
			}
		}
	}
	return ""
}

// validateHTTPURL проверяет, что адрес — обычная веб-ссылка.
func validateHTTPURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if len(value) > 2000 {
		return "", errors.New("слишком длинный")
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return "", errors.New("указан неверно")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("должен начинаться с http:// или https://")
	}
	if parsed.Host == "" {
		return "", errors.New("не указан адрес сайта")
	}
	return parsed.String(), nil
}

// SetVerified отмечает продавца проверенным.
func (s *Sellers) SetVerified(ctx context.Context, sellerID uuid.UUID, viewer Viewer, verified bool) error {
	if err := s.sellers.SetVerified(ctx, sellerID, viewer.UserID, verified); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return apierr.NotFound("Продавец")
		}
		return apierr.Internal(err)
	}

	s.recordAudit(ctx, viewer, "seller.verify", sellerID.String(),
		map[string]any{"verified": verified})
	return nil
}

// SetActive включает или отключает карточку.
func (s *Sellers) SetActive(ctx context.Context, sellerID uuid.UUID, viewer Viewer, active bool) error {
	err := s.sellers.SetActive(
		ctx, sellerID, viewer.UserID, viewer.Role == domain.RoleAdmin, active)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return apierr.NotFound("Продавец")
		}
		return apierr.Internal(err)
	}

	s.recordAudit(ctx, viewer, "seller.active", sellerID.String(),
		map[string]any{"active": active})
	return nil
}

// SaveLink сохраняет приватную заметку дилера о продавце.
func (s *Sellers) SaveLink(ctx context.Context, sellerID uuid.UUID, dealerID uuid.UUID, note string, trusted bool) error {
	if _, err := s.sellers.ByID(ctx, sellerID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return apierr.NotFound("Продавец")
		}
		return apierr.Internal(err)
	}

	if err := s.sellers.UpsertLink(ctx, dealerID, sellerID, normalizeShortText(note, 2000), trusted); err != nil {
		return apierr.Internal(err)
	}
	return nil
}

// DeleteLink удаляет заметку дилера.
func (s *Sellers) DeleteLink(ctx context.Context, sellerID, dealerID uuid.UUID) error {
	if err := s.sellers.DeleteLink(ctx, dealerID, sellerID); err != nil {
		return apierr.Internal(err)
	}
	return nil
}

// Facets возвращает значения для фильтров справочника.
func (s *Sellers) Facets(ctx context.Context, countries []string) (map[string]any, error) {
	regions, err := s.sellers.Regions(ctx, countries)
	if err != nil {
		return nil, apierr.Internal(err)
	}

	brands, err := s.sellers.SellerBrands(ctx, countries)
	if err != nil {
		return nil, apierr.Internal(err)
	}

	return map[string]any{
		"dictionaries": domain.SellerDictionaries(),
		"regions":      regions,
		"brands":       brands,
	}, nil
}

func (s *Sellers) recordAudit(ctx context.Context, viewer Viewer, action, entityID string, diff map[string]any) {
	actorID := viewer.UserID
	if err := s.audit.RecordAudit(ctx, store.AuditEntry{
		ActorID:   &actorID,
		ActorRole: string(viewer.Role),
		Action:    action,
		Entity:    "seller",
		EntityID:  entityID,
		Diff:      diff,
	}); err != nil {
		s.log.ErrorContext(ctx, "не удалось записать аудит продавца", "error", err, "action", action)
	}
}
