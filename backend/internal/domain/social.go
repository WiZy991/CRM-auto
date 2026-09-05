package domain

// SocialNetwork — канал автопостинга объявлений.
type SocialNetwork string

const (
	NetworkTelegram  SocialNetwork = "telegram"
	NetworkVK        SocialNetwork = "vk"
	NetworkWhatsApp  SocialNetwork = "whatsapp"
	NetworkInstagram SocialNetwork = "instagram"
	NetworkYouTube   SocialNetwork = "youtube"
	NetworkRuTube    SocialNetwork = "rutube"
)

// SocialNetworkOrder — порядок карточек в кабинете дилера.
var SocialNetworkOrder = []SocialNetwork{
	NetworkTelegram, NetworkVK, NetworkWhatsApp,
	NetworkInstagram, NetworkYouTube, NetworkRuTube,
}

func (n SocialNetwork) Valid() bool {
	switch n {
	case NetworkTelegram, NetworkVK, NetworkWhatsApp, NetworkInstagram, NetworkYouTube, NetworkRuTube:
		return true
	default:
		return false
	}
}

func (n SocialNetwork) Title() string {
	switch n {
	case NetworkTelegram:
		return "Telegram"
	case NetworkVK:
		return "ВКонтакте"
	case NetworkWhatsApp:
		return "WhatsApp"
	case NetworkInstagram:
		return "Instagram"
	case NetworkYouTube:
		return "YouTube"
	case NetworkRuTube:
		return "RuTube"
	default:
		return string(n)
	}
}

// AuthKind — как дилер подключает канал.
func (n SocialNetwork) AuthKind() string {
	switch n {
	case NetworkInstagram, NetworkYouTube:
		return "oauth"
	default:
		return "keys"
	}
}

// SocialAccountStatus — состояние подключения.
type SocialAccountStatus string

const (
	SocialDisconnected SocialAccountStatus = "disconnected"
	SocialConnected    SocialAccountStatus = "connected"
	SocialNeedsReauth  SocialAccountStatus = "needs_reauth"
	SocialError        SocialAccountStatus = "error"
)

func (s SocialAccountStatus) Valid() bool {
	switch s {
	case SocialDisconnected, SocialConnected, SocialNeedsReauth, SocialError:
		return true
	default:
		return false
	}
}

func ParseSocialNetwork(raw string) (SocialNetwork, bool) {
	n := SocialNetwork(raw)
	return n, n.Valid()
}
