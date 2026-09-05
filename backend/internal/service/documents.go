package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/autoimport/crm/internal/domain"
	"github.com/autoimport/crm/internal/pkg/apierr"
	"github.com/autoimport/crm/internal/pkg/docs"
	"github.com/autoimport/crm/internal/store"
)

// PreviewDocument собирает HTML формы без записи в сделку.
func (p *Pipeline) PreviewDocument(ctx context.Context, dealID uuid.UUID, viewer Viewer, kindRaw string) ([]byte, []string, string, error) {
	if _, err := p.requireManage(ctx, dealID, viewer); err != nil {
		return nil, nil, "", err
	}
	kind := store.DocumentKind(normalizeEnum(kindRaw))
	if !kind.Valid() || kind == store.DocOther {
		return nil, nil, "", apierr.BadRequest("Этот вид нельзя сформировать автоматически")
	}

	payload, err := p.docPayload(ctx, dealID, viewer, kind)
	if err != nil {
		return nil, nil, "", err
	}
	html, err := docs.Render(kind, payload)
	if err != nil {
		return nil, nil, "", apierr.Internal(err)
	}
	return html, payload.Warnings, kind.Title(), nil
}

// GenerateDocuments пишет печатные формы в сделку, заполняя поля клиента,
// дилера, автомобиля и сумм. Пустой список видов — весь комплект.
func (p *Pipeline) GenerateDocuments(ctx context.Context, dealID uuid.UUID, viewer Viewer, kinds []string, visibleToClient bool) ([]store.Document, []string, error) {
	if _, err := p.requireManage(ctx, dealID, viewer); err != nil {
		return nil, nil, err
	}
	if p.disk == nil {
		return nil, nil, apierr.Unavailable("Хранилище файлов недоступно")
	}

	selected := make([]store.DocumentKind, 0, len(docs.PrintableKinds()))
	if len(kinds) == 0 {
		selected = append(selected, docs.PrintableKinds()...)
	} else {
		for _, raw := range kinds {
			kind := store.DocumentKind(normalizeEnum(raw))
			if !kind.Valid() || kind == store.DocOther {
				return nil, nil, apierr.BadRequest("Неизвестный или непечатный вид документа")
			}
			selected = append(selected, kind)
		}
	}

	payload, err := p.docPayload(ctx, dealID, viewer, selected[0])
	if err != nil {
		return nil, nil, err
	}

	created := make([]store.Document, 0, len(selected))
	for _, kind := range selected {
		payload.Kind = string(kind)
		payload.Title = kind.Title()
		html, err := docs.Render(kind, payload)
		if err != nil {
			return created, payload.Warnings, apierr.Internal(err)
		}
		saved, err := p.disk.SavePrivate("docs", "html", html, "text/html; charset=utf-8")
		if err != nil {
			return created, payload.Warnings, apierr.Internal(err)
		}
		title := fmt.Sprintf("%s, сделка № %s", kind.Title(), payload.DealNumber)
		doc, err := p.comms.AddDocument(ctx, store.AddDocumentParams{
			DealID:          dealID,
			Kind:            kind,
			Title:           title,
			FileURL:         saved.RelPath,
			MimeType:        saved.MimeType,
			Bytes:           saved.Bytes,
			UploadedBy:      viewer.UserID,
			VisibleToClient: visibleToClient,
			IsAdmin:         viewer.Role == domain.RoleAdmin,
		})
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return created, payload.Warnings, apierr.NotFound("Сделка")
			}
			return created, payload.Warnings, apierr.Internal(err)
		}
		created = append(created, *doc)
	}

	p.recordAudit(ctx, viewer.UserID, "deal.document.generate", dealID.String(),
		map[string]any{"count": len(created)})

	if visibleToClient && len(created) > 0 {
		deal, err := p.deals.ByIDForParticipant(ctx, dealID, viewer.UserID, viewer.Role == domain.RoleAdmin)
		if err == nil {
			if nerr := p.notify.Create(ctx, store.CreateNotificationParams{
				UserID: deal.ClientID,
				Kind:   store.NotifyDealStageChanged,
				Title:  "Сформированы документы по сделке",
				Body:   fmt.Sprintf("Готово форм: %d", len(created)),
				Link:   fmt.Sprintf("/app/deals/%s", dealID),
			}); nerr != nil {
				p.log.ErrorContext(ctx, "не удалось уведомить о пакете документов", "error", nerr)
			}
		}
	}

	return created, payload.Warnings, nil
}

func (p *Pipeline) docPayload(ctx context.Context, dealID uuid.UUID, viewer Viewer, kind store.DocumentKind) (docs.Payload, error) {
	access, err := p.access(ctx, dealID, viewer)
	if err != nil {
		return docs.Payload{}, err
	}
	deal := access.Deal

	client, err := p.users.ByID(ctx, deal.ClientID)
	if err != nil {
		return docs.Payload{}, apierr.Internal(fmt.Errorf("клиент сделки: %w", err))
	}
	dealerUser, err := p.users.ByID(ctx, deal.DealerID)
	if err != nil {
		return docs.Payload{}, apierr.Internal(fmt.Errorf("дилер сделки: %w", err))
	}

	var dealerProfile *store.DealerProfile
	if profile, err := p.dealers.ByUserID(ctx, deal.DealerID); err == nil {
		dealerProfile = profile
	} else if !errors.Is(err, store.ErrNotFound) {
		return docs.Payload{}, apierr.Internal(err)
	}

	var car *domain.Car
	if deal.CarID != nil {
		item, err := p.cars.ByID(ctx, *deal.CarID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return docs.Payload{}, apierr.Internal(err)
		}
		car = item
	}

	var seller *domain.Seller
	if deal.SellerID != nil {
		item, err := p.sellers.ByID(ctx, *deal.SellerID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return docs.Payload{}, apierr.Internal(err)
		}
		seller = item
	}

	passport, address := p.decryptParty(ctx, deal.ClientID)
	return docs.FromSources(kind, deal, client, dealerUser, dealerProfile, car, seller, passport, address, time.Now()), nil
}

func (p *Pipeline) decryptParty(ctx context.Context, userID uuid.UUID) (string, string) {
	if p.cipher == nil || p.users == nil {
		return "", ""
	}
	raw, err := p.users.EncryptedPII(ctx, userID)
	if err != nil {
		return "", ""
	}
	passport, err := p.cipher.Decrypt(raw.Passport)
	if err != nil {
		passport = ""
	}
	address, err := p.cipher.Decrypt(raw.Address)
	if err != nil {
		address = ""
	}
	return passport, address
}
