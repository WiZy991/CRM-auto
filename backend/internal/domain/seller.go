package domain

import (
	"time"

	"github.com/google/uuid"
)

// SellerKind — тип зарубежного поставщика.
//
// Различие существенное: у аукциона есть лот и оценка состояния, у
// экспортёра — склад и сроки отгрузки, у завода — минимальная партия.
// Интерфейс подбора опирается на этот тип, поэтому он вынесен в enum, а не
// в свободное текстовое поле.
type SellerKind string

const (
	SellerAuction    SellerKind = "auction"
	SellerExporter   SellerKind = "exporter"
	SellerDealership SellerKind = "dealership"
	SellerFactory    SellerKind = "factory"
	SellerBroker     SellerKind = "broker"
)

var sellerKindTitles = map[SellerKind]string{
	SellerAuction:    "Аукцион",
	SellerExporter:   "Экспортёр",
	SellerDealership: "Автосалон",
	SellerFactory:    "Завод",
	SellerBroker:     "Брокер",
}

func (k SellerKind) Valid() bool { _, ok := sellerKindTitles[k]; return ok }

func (k SellerKind) Title() string {
	if title, ok := sellerKindTitles[k]; ok {
		return title
	}
	return string(k)
}

// SellerKindOrder задаёт порядок в справочнике.
var SellerKindOrder = []SellerKind{
	SellerAuction, SellerExporter, SellerDealership, SellerFactory, SellerBroker,
}

// SellerDictionaries возвращает справочники для фильтров базы продавцов.
func SellerDictionaries() map[string][]DictionaryEntry {
	return map[string][]DictionaryEntry{
		"kind": dictionary(sellerKindTitles, SellerKindOrder),
		"country": {
			{Value: string(OriginChina), Title: OriginChina.Title()},
			{Value: string(OriginJapan), Title: OriginJapan.Title()},
		},
	}
}

// Seller — зарубежный поставщик автомобилей.
type Seller struct {
	ID uuid.UUID

	// UserID заполнен, если продавец сам зарегистрировался на платформе.
	UserID    *uuid.UUID
	CreatedBy *uuid.UUID

	Country Origin
	Kind    SellerKind

	Name      string
	NameLocal string
	Region    string
	City      string
	Address   string

	Brands      []string
	Description string
	Website     string
	// Contacts хранит разнородные поля: у японского аукциона это логин и
	// идентификатор участника, у китайского экспортёра — WeChat и телефон.
	Contacts map[string]string
	LogoURL  string

	MinOrderQty           int
	ExportExperienceYears *int

	RatingAvg   float64
	RatingCount int

	VerifiedAt *time.Time
	IsActive   bool

	CreatedAt time.Time
	UpdatedAt time.Time
}

// IsVerified сообщает, проверен ли продавец администрацией.
func (s *Seller) IsVerified() bool { return s.VerifiedAt != nil }

// OwnedBy проверяет, принадлежит ли карточка продавца пользователю.
func (s *Seller) OwnedBy(userID uuid.UUID) bool {
	return s.UserID != nil && *s.UserID == userID
}

// DisplayName собирает название с локальным написанием.
//
// Локальное имя показывается рядом с латинским: японские аукционы ищутся
// по обоим написаниям, и без иероглифов карточку не найти.
func (s *Seller) DisplayName() string {
	if s.NameLocal == "" || s.NameLocal == s.Name {
		return s.Name
	}
	return s.Name + " (" + s.NameLocal + ")"
}

// PublicContacts отдаёт контакты только дилеру и администратору.
// Гостю каталога телефон и WeChat не показываем.
func (s *Seller) PublicContacts(viewerIsDealer bool) map[string]string {
	if !viewerIsDealer {
		return map[string]string{}
	}
	if s.Contacts == nil {
		return map[string]string{}
	}
	return s.Contacts
}
