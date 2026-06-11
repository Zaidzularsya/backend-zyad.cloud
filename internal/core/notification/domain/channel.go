package domain

type Channel string

const (
	ChannelEmail    Channel = "email"
	ChannelWhatsApp Channel = "whatsapp"
	ChannelInApp    Channel = "in_app"
	ChannelDiscord  Channel = "discord"
)

func (c Channel) IsValid() bool {
	switch c {
	case ChannelEmail,
		ChannelWhatsApp,
		ChannelInApp,
		ChannelDiscord:
		return true
	default:
		return false
	}
}

func SupportedChannels() []Channel {
	return []Channel{
		ChannelEmail,
		ChannelWhatsApp,
		ChannelInApp,
		ChannelDiscord,
	}
}
