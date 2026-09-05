package domain

import (
	"strings"
	"testing"
	"time"
)

func TestValidateBannerHrefRejectsDangerousSchemes(t *testing.T) {
	// Ссылка баннера подставляется в атрибут href на публичной странице.
	// Схемы javascript: и data: там означают выполнение чужого кода в
	// браузере посетителя, поэтому проверка обязана их отсекать.
	dangerous := []string{
		"javascript:alert(1)",
		"JavaScript:alert(1)",
		"  javascript:alert(1)  ",
		"data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==",
		"vbscript:msgbox(1)",
		"file:///etc/passwd",
		"ftp://example.com/file",
		"//example.com/path",
		"/relative/path",
		"example.com",
		"",
		"   ",
	}

	for _, raw := range dangerous {
		if _, err := ValidateBannerHref(raw); err == nil {
			t.Errorf("ValidateBannerHref(%q) приняла недопустимую ссылку", raw)
		}
	}
}

func TestValidateBannerHrefAcceptsWebLinks(t *testing.T) {
	valid := []string{
		"https://example.com",
		"http://example.com/promo",
		"https://example.com/promo?utm_source=crm&utm_medium=banner",
		"https://sub.example.co.jp/лот/12345",
	}

	for _, raw := range valid {
		if _, err := ValidateBannerHref(raw); err != nil {
			t.Errorf("ValidateBannerHref(%q) отклонила корректную ссылку: %v", raw, err)
		}
	}
}

func TestValidateBannerHrefRejectsOverlyLongLink(t *testing.T) {
	long := "https://example.com/" + strings.Repeat("a", 2100)

	if _, err := ValidateBannerHref(long); err == nil {
		t.Error("слишком длинная ссылка должна быть отклонена")
	}
}

func TestValidateBannerPeriod(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name     string
		startsAt time.Time
		endsAt   time.Time
		wantErr  bool
	}{
		{
			name:     "обычная недельная кампания",
			startsAt: now,
			endsAt:   now.Add(7 * 24 * time.Hour),
		},
		{
			name:     "запуск запланирован на будущее",
			startsAt: now.Add(30 * 24 * time.Hour),
			endsAt:   now.Add(60 * 24 * time.Hour),
		},
		{
			name:     "окончание раньше начала",
			startsAt: now.Add(24 * time.Hour),
			endsAt:   now,
			wantErr:  true,
		},
		{
			name:     "нулевая длительность",
			startsAt: now,
			endsAt:   now,
			wantErr:  true,
		},
		{
			name:     "период уже прошёл",
			startsAt: now.Add(-48 * time.Hour),
			endsAt:   now.Add(-24 * time.Hour),
			wantErr:  true,
		},
		{
			name:     "короче минимального часа",
			startsAt: now,
			endsAt:   now.Add(30 * time.Minute),
			wantErr:  true,
		},
		{
			name:     "дольше года",
			startsAt: now,
			endsAt:   now.Add(400 * 24 * time.Hour),
			wantErr:  true,
		},
		{
			name:     "старт слишком далеко в будущем",
			startsAt: now.Add(200 * 24 * time.Hour),
			endsAt:   now.Add(210 * 24 * time.Hour),
			wantErr:  true,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := ValidateBannerPeriod(testCase.startsAt, testCase.endsAt, now)
			if testCase.wantErr && err == nil {
				t.Error("ожидалась ошибка, но период принят")
			}
			if !testCase.wantErr && err != nil {
				t.Errorf("период должен быть принят, получено: %v", err)
			}
		})
	}
}

func TestBannerIsShowable(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	base := Banner{
		Status:   BannerActive,
		StartsAt: now.Add(-time.Hour),
		EndsAt:   now.Add(time.Hour),
	}

	if !base.IsShowable(now) {
		t.Error("действующий баннер должен показываться")
	}

	notStarted := base
	notStarted.StartsAt = now.Add(time.Hour)
	notStarted.EndsAt = now.Add(2 * time.Hour)
	if notStarted.IsShowable(now) {
		t.Error("баннер до начала периода показываться не должен")
	}

	// Момент окончания — граница исключающая: иначе баннер живёт лишний
	// такт после истечения оплаченного периода.
	justEnded := base
	justEnded.EndsAt = now
	if justEnded.IsShowable(now) {
		t.Error("баннер в момент окончания периода показываться не должен")
	}

	for _, status := range []BannerStatus{
		BannerDraft, BannerModeration, BannerPaused, BannerRejected, BannerExpired,
	} {
		candidate := base
		candidate.Status = status
		if candidate.IsShowable(now) {
			t.Errorf("баннер в состоянии %q показываться не должен", status)
		}
	}
}

func TestBannerCTR(t *testing.T) {
	empty := Banner{}
	if empty.CTR() != 0 {
		t.Error("без показов доля кликов должна быть нулевой, а не делением на ноль")
	}

	banner := Banner{Impressions: 400, Clicks: 10}
	if got := banner.CTR(); got != 0.025 {
		t.Errorf("CTR = %v, ожидалось 0.025", got)
	}
}
