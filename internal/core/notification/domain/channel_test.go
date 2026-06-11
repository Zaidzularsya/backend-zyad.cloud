package domain

import "testing"

func TestChannelIsValid(t *testing.T) {
	valid := []Channel{
		ChannelEmail,
		ChannelWhatsApp,
		ChannelInApp,
		ChannelDiscord,
	}

	for _, channel := range valid {
		if !channel.IsValid() {
			t.Fatalf("expected channel %q to be valid", channel)
		}
	}

	if Channel("sms").IsValid() {
		t.Fatal("expected unsupported channel to be invalid")
	}
}
