package mail

import (
	"fmt"
	"html"
	"net/url"
	"strings"
)

func VerificationMessage(publicURL, recipient, displayName, token string) Message {
	link := actionURL(publicURL, "/verify-email", token)
	name := strings.TrimSpace(displayName)
	if name == "" {
		name = "слушатель"
	}
	return Message{
		Kind:    "verify_email",
		To:      recipient,
		Subject: "Подтвердите почту в Mixora",
		TextBody: fmt.Sprintf(
			"Привет, %s!\n\nПодтвердите адрес электронной почты:\n%s\n\nЕсли вы не создавали аккаунт Mixora, просто проигнорируйте это письмо.",
			name,
			link,
		),
		HTMLBody: fmt.Sprintf(
			`<p>Привет, %s!</p><p><a href="%s">Подтвердить адрес электронной почты</a></p><p>Если вы не создавали аккаунт Mixora, просто проигнорируйте это письмо.</p>`,
			html.EscapeString(name),
			html.EscapeString(link),
		),
	}
}

func PasswordResetMessage(publicURL, recipient, displayName, token string) Message {
	link := actionURL(publicURL, "/reset-password", token)
	name := strings.TrimSpace(displayName)
	if name == "" {
		name = "слушатель"
	}
	return Message{
		Kind:    "password_reset",
		To:      recipient,
		Subject: "Сброс пароля Mixora",
		TextBody: fmt.Sprintf(
			"Привет, %s!\n\nЧтобы задать новый пароль, откройте ссылку:\n%s\n\nЕсли вы не запрашивали сброс, ничего делать не нужно.",
			name,
			link,
		),
		HTMLBody: fmt.Sprintf(
			`<p>Привет, %s!</p><p><a href="%s">Задать новый пароль</a></p><p>Если вы не запрашивали сброс, ничего делать не нужно.</p>`,
			html.EscapeString(name),
			html.EscapeString(link),
		),
	}
}

func actionURL(publicURL, path, token string) string {
	base := strings.TrimRight(publicURL, "/") + path
	query := url.Values{"token": []string{token}}
	return base + "?" + query.Encode()
}
