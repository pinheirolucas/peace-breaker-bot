package bot

type Option func(*Bot)

func WithOwner(owner string) Option {
	return func(b *Bot) {
		b.owner = owner
	}
}

// WithLocale fixes the bot's response language regardless of the invoking
// guild's own locale. Empty (the default) leaves per-guild detection in place.
func WithLocale(locale string) Option {
	return func(b *Bot) {
		b.locale = locale
	}
}

// WithVersion sets the version string logged when the bot starts.
func WithVersion(version string) Option {
	return func(b *Bot) {
		b.version = version
	}
}
