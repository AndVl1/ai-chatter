package telegram

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

type sender interface {
	Send(c tgbotapi.Chattable) (tgbotapi.Message, error)
}

type botAPISender struct{ api *tgbotapi.BotAPI }

func (s botAPISender) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	return s.api.Send(c)
}
