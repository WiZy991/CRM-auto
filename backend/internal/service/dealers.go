package service

import (
	"context"
	"errors"
	"strings"
	"unicode"

	"github.com/google/uuid"

	"github.com/autoimport/crm/internal/domain"
	"github.com/autoimport/crm/internal/pkg/apierr"
	"github.com/autoimport/crm/internal/store"
)

// PublicDealers — публичный каталог импортёров и профиль владельца.
type PublicDealers struct {
	dealers *store.Dealers
	cars    *store.Cars
	reviews *store.Reviews
}

func NewPublicDealers(dealers *store.Dealers, cars *store.Cars, reviews *store.Reviews) *PublicDealers {
	return &PublicDealers{dealers: dealers, cars: cars, reviews: reviews}
}

// List возвращает страницу витрины дилеров.
func (s *PublicDealers) List(ctx context.Context, city string, limit, offset int) ([]store.PublicDealer, int, error) {
	items, total, err := s.dealers.List(ctx, store.DealerFilter{
		City: city, Limit: limit, Offset: offset,
	})
	if err != nil {
		return nil, 0, apierr.Internal(err)
	}
	return items, total, nil
}

// BySlug возвращает публичную карточку.
func (s *PublicDealers) BySlug(ctx context.Context, slug string) (*store.PublicDealer, error) {
	item, err := s.dealers.BySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, apierr.NotFound("Дилер")
		}
		return nil, apierr.Internal(err)
	}
	return item, nil
}

// CarsBySlug — объявления дилера, которые можно показать гостю.
func (s *PublicDealers) CarsBySlug(ctx context.Context, slug string, viewerID uuid.UUID) (*store.CarPage, error) {
	dealer, err := s.BySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	page, err := s.cars.List(ctx, store.CarFilter{
		DealerID: &dealer.UserID,
		Limit:    24,
		Sort:     "fresh",
	}, viewerID, true)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	return page, nil
}

// Mine возвращает профиль текущего дилера.
func (s *PublicDealers) Mine(ctx context.Context, userID uuid.UUID, role domain.Role) (*store.DealerProfile, error) {
	if role != domain.RoleDealer && role != domain.RoleAdmin {
		return nil, apierr.Forbidden("Профиль импортёра доступен дилеру")
	}
	profile, err := s.dealers.ByUserID(ctx, userID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			if ensureErr := s.dealers.EnsureDefault(ctx, userID, ""); ensureErr != nil {
				return nil, apierr.Internal(ensureErr)
			}
			profile, err = s.dealers.ByUserID(ctx, userID)
		}
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, apierr.NotFound("Профиль дилера")
			}
			return nil, apierr.Internal(err)
		}
	}
	return profile, nil
}

// SaveMine сохраняет карточку дилера.
func (s *PublicDealers) SaveMine(ctx context.Context, userID uuid.UUID, role domain.Role, write store.DealerWrite) (*store.DealerProfile, error) {
	if role != domain.RoleDealer && role != domain.RoleAdmin {
		return nil, apierr.Forbidden("Профиль импортёра доступен дилеру")
	}

	if _, err := s.dealers.ByUserID(ctx, userID); errors.Is(err, store.ErrNotFound) {
		if ensureErr := s.dealers.EnsureDefault(ctx, userID, write.CompanyName); ensureErr != nil {
			return nil, apierr.Internal(ensureErr)
		}
	}

	write.Slug = strings.ToLower(strings.TrimSpace(write.Slug))
	write.CompanyName = strings.TrimSpace(write.CompanyName)
	write.City = strings.TrimSpace(write.City)
	write.Description = strings.TrimSpace(write.Description)
	write.LegalName = strings.TrimSpace(write.LegalName)
	write.INN = digitsOnly(write.INN)
	write.Website = strings.TrimSpace(write.Website)
	write.Address = strings.TrimSpace(write.Address)

	details := map[string]string{}
	if !store.SlugOK(write.Slug) {
		details["slug"] = "адрес карточки: латиница, цифры и дефис, от 2 до 63 символов"
	}
	if len([]rune(write.CompanyName)) < 2 {
		details["company_name"] = "укажите название компании"
	}
	if write.City == "" {
		details["city"] = "укажите город"
	}
	if write.INN != "" && len(write.INN) != 10 && len(write.INN) != 12 {
		details["inn"] = "ИНН — 10 или 12 цифр"
	}
	if len(details) > 0 {
		return nil, apierr.Validation(details)
	}

	countries := make([]string, 0, len(write.WorkCountries))
	for _, country := range write.WorkCountries {
		switch strings.ToLower(country) {
		case "cn", "jp":
			countries = append(countries, strings.ToLower(country))
		}
	}
	write.WorkCountries = countries

	profile, err := s.dealers.Upsert(ctx, userID, write)
	if err != nil {
		if errors.Is(err, store.ErrDuplicate) {
			return nil, apierr.Conflict("Этот адрес карточки уже занят")
		}
		return nil, apierr.Internal(err)
	}
	return profile, nil
}

func (s *PublicDealers) ReviewsBySlug(ctx context.Context, slug string) ([]store.Review, error) {
	dealer, err := s.BySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if s.reviews == nil {
		return nil, nil
	}
	items, err := s.reviews.ListByDealer(ctx, dealer.UserID, true, 50)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	return items, nil
}

func (s *PublicDealers) MineReviews(ctx context.Context, userID uuid.UUID, role domain.Role) ([]store.Review, error) {
	if role != domain.RoleDealer && role != domain.RoleAdmin {
		return nil, apierr.Forbidden("Отзывы доступны дилеру")
	}
	if s.reviews == nil {
		return nil, nil
	}
	items, err := s.reviews.ListByDealer(ctx, userID, false, 50)
	if err != nil {
		return nil, apierr.Internal(err)
	}
	return items, nil
}

func (s *PublicDealers) ReplyReview(ctx context.Context, reviewID, dealerID uuid.UUID, role domain.Role, reply string) (*store.Review, error) {
	if role != domain.RoleDealer && role != domain.RoleAdmin {
		return nil, apierr.Forbidden("Ответ на отзыв может оставить дилер")
	}
	text := strings.TrimSpace(reply)
	if text == "" {
		return nil, apierr.BadRequest("Напишите ответ")
	}
	if len([]rune(text)) > 2000 {
		return nil, apierr.Validation(map[string]string{"reply": "слишком длинный текст"})
	}
	rev, err := s.reviews.Reply(ctx, reviewID, dealerID, text)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, apierr.NotFound("Отзыв")
		}
		return nil, apierr.Internal(err)
	}
	return rev, nil
}

// EnsureDefault создаёт каркас профиля при регистрации.
func (s *PublicDealers) EnsureDefault(ctx context.Context, userID uuid.UUID, fullName string) error {
	if err := s.dealers.EnsureDefault(ctx, userID, fullName); err != nil {
		return apierr.Internal(err)
	}
	return nil
}

func digitsOnly(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
