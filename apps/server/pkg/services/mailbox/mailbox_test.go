package mailbox

import "testing"

func TestConfigAddress(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		want    string
		wantErr bool
	}{
		{name: "gmail preset", cfg: Config{Provider: "gmail"}, want: "imap.gmail.com:993"},
		{name: "outlook preset", cfg: Config{Provider: "outlook"}, want: "outlook.office365.com:993"},
		{name: "case-insensitive provider", cfg: Config{Provider: "Gmail"}, want: "imap.gmail.com:993"},
		{name: "preset host override", cfg: Config{Provider: "gmail", Host: "proxy.local", Port: 1993}, want: "proxy.local:1993"},
		{name: "custom with host and port", cfg: Config{Provider: "custom", Host: "imap.primer.rs", Port: 993}, want: "imap.primer.rs:993"},
		{name: "custom defaults port", cfg: Config{Provider: "custom", Host: "imap.primer.rs"}, want: "imap.primer.rs:993"},
		{name: "empty provider treated as custom", cfg: Config{Host: "imap.primer.rs"}, want: "imap.primer.rs:993"},
		{name: "custom without host errors", cfg: Config{Provider: "custom"}, wantErr: true},
		{name: "empty provider without host errors", cfg: Config{}, wantErr: true},
		{name: "unknown provider errors", cfg: Config{Provider: "protonmail"}, wantErr: true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := c.cfg.Address()
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got address %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Errorf("Address() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestMailboxOrDefault(t *testing.T) {
	if got := (Config{}).MailboxOrDefault(); got != DefaultMailbox {
		t.Errorf("MailboxOrDefault() = %q, want %q", got, DefaultMailbox)
	}
	if got := (Config{Mailbox: "  "}).MailboxOrDefault(); got != DefaultMailbox {
		t.Errorf("MailboxOrDefault() with blanks = %q, want %q", got, DefaultMailbox)
	}
	if got := (Config{Mailbox: "Archive"}).MailboxOrDefault(); got != "Archive" {
		t.Errorf("MailboxOrDefault() = %q, want Archive", got)
	}
}
