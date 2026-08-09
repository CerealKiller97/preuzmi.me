package config

import "strings"

// FriendlyError turns Validate / load errors into short Serbian messages for the UI.
func FriendlyError(err error) string {
	if err == nil {
		return ""
	}

	msg := err.Error()

	switch {
	case strings.Contains(msg, "application.port"):
		return "Port aplikacije mora biti između 0 i 65535."
	case strings.Contains(msg, "invalid storage"):
		return "Tip skladišta mora biti „local“ ili „s3“."
	case strings.Contains(msg, "s3.bucket is empty"):
		return "Za S3 skladište obavezno je ime bucket-a."
	case strings.Contains(msg, "s3.access_key or s3.secret_key"):
		return "Za S3 skladište obavezni su access key i secret key."
	case strings.Contains(msg, "download_path is empty"):
		return "Folder za preuzimanje ne sme biti prazan."
	case strings.Contains(msg, "check_until"):
		return "Dan za osvežavanje mora biti između 1 i 31."
	case strings.Contains(msg, "invalid log_level"):
		return "Nivo logovanja nije ispravan (debug, info, warn, error…)."
	case strings.Contains(msg, "invalid lang"):
		return "Pismo mora biti „latin“ ili „cyrillic“."
	case strings.Contains(msg, "notifications.mode"):
		return "Režim obaveštenja mora biti off, per_receipt ili all_done."
	case strings.Contains(msg, "notifications.driver"):
		return "Drajver obaveštenja mora biti smtp ili telegram."
	case strings.Contains(msg, "smtp.host is empty"):
		return "Za SMTP obaveštenja obavezan je host."
	case strings.Contains(msg, "smtp.from or smtp.to"):
		return "Za SMTP obaveštenja obavezna su polja Od i Za."
	case strings.Contains(msg, "telegram.bot_token or telegram.chat_id"):
		return "Za Telegram obaveštenja obavezni su bot token i chat ID."
	case strings.Contains(msg, "invalid character") || strings.Contains(msg, "cannot unmarshal"):
		return "Podaci nisu u ispravnom formatu."
	default:
		return msg
	}
}
