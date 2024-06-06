package telegram

import (
	"fmt"
	"github.com/LightningTipBot/LightningTipBot/internal"
	"github.com/LightningTipBot/LightningTipBot/internal/errors"
	"time"

	//"github.com/LightningTipBot/LightningTipBot/internal/lnbits"
	"github.com/LightningTipBot/LightningTipBot/internal/telegram/intercept"
	log "github.com/sirupsen/logrus"

	"github.com/almerlucke/go-iban/iban"
	tb "gopkg.in/lightningtipbot/telebot.v3"
)

func (bot *TipBot) buyHandler(ctx intercept.Context) (intercept.Context, error) {
	// commands: /buy IBAN
	m := ctx.Message()
	giveniban, err := getArgumentFromCommand(ctx.Message().Text, 1)
	if m.Chat.Type != tb.ChatPrivate {
		return ctx, errors.Create(errors.NoPrivateChatError)
	}
	log.Infof("[buyHandler] %s", m.Text)
	if err != nil {
		NewMessage(ctx.Message(), WithDuration(0, bot))
		bot.trySendMessage(ctx.Sender(), Translate(ctx, "buyHelpText"))
		errmsg := fmt.Sprintf("[/buyHandler] Error: Could not getArgumentFromCommand: %s", err.Error())
		log.Errorln(errmsg)
		return ctx, errors.New(errors.InvalidSyntaxError, err)
	}
	user := LoadUser(ctx)

	// load user settings
	user, err = GetLnbitsUserWithSettings(user.Telegram, *bot)
	if user.Wallet == nil {
		return ctx, errors.Create(errors.UserNoWalletError)
	}
	userStr := GetUserStr(user.Telegram)

	if m.Chat.Type != tb.ChatPrivate {
		// delete message
		bot.tryDeleteMessage(m)
		return ctx, errors.Create(errors.NoPrivateChatError)
	}
	iban, err := iban.NewIBAN(giveniban)

	if err != nil {
		fmt.Printf("%v\n", err)
		//bot.trySendMessage(ctx.Sender(), Translate(ctx, "activateScrubHelpText"))
		errmsg := fmt.Sprintf("[/buy] Error: invalid IBAN provided: %s", err.Error())
		log.Errorln(errmsg)
		bot.trySendMessage(ctx.Sender(), Translate(ctx, "invalidIBANHelpText"))
		return ctx, errors.New(errors.InvalidSyntaxError, err)
	}
	log.Infof("[buyHandler] valid iban provided: %s", iban.Code)

	// get user's lnaddress
	fromUser := LoadUser(ctx)
	lnaddr, _ := bot.UserGetLightningAddress(fromUser)

	// default amount to purchase in fiat
	purchaseAmount := "200"

	// send user confirmation message
	bot.trySendMessage(m.Sender, fmt.Sprintf("Buy invoked\n\nUser: %s\nIBAN: %s\nRecipient: %s\nAmount fiat: %s", userStr, iban.Code, lnaddr, purchaseAmount))

	// logging purchase details
	log.Infof("[buyHandler] buy details: %s %s %s %s", userStr, iban.Code, lnaddr, purchaseAmount)

	// generate the order
	voucherbotManager := &VoucherBot{APIKey: internal.Configuration.Voucherbot.ApiKey}

	voucherbotManager.setLightningRecipient(lnaddr, purchaseAmount, iban.Code)
	orderResult := voucherbotManager.createLightningOrder()

	paymentMethod, _ := orderResult["payment_method"].(map[string]interface{})
	creditorName, _ := paymentMethod["creditor_name"].(string)
	creditorAddress, _ := paymentMethod["creditor_address"].(string)
	creditorBankName, _ := paymentMethod["creditor_bank_name"].(string)
	creditorBankIban, _ := paymentMethod["creditor_bank_iban"].(string)
	creditorBankBic, _ := paymentMethod["creditor_bank_bic"].(string)
	now := time.Now()
	currency := internal.Configuration.Pos.Currency

	if orderResult["status"].(string) == "order.accepted" {
		orderConfirmation := fmt.Sprintf("✔️ORDER RECEIVED.\n\n"+
			"Date: %s"+
			"\nFiat Amount: %s %s"+
			"\nOrder type: Lightning Push"+
			"\n\nSEPA Bank transfer coordinates"+
			"\nBeneficiary: `%s`"+
			"\nAddress: `%s`"+
			"\nBank: `%s`"+
			"\nIBAN: `%s`"+
			"\nBIC: `%s`"+
			"\nNet amount to pay: %s %s"+
			"\nPayment reason: `%s`"+
			"\n\nFrom IBAN: %s"+
			"\n\nOrderid: `%s`", now.Format("2006-01-02"), purchaseAmount, currency, creditorName, creditorAddress, creditorBankName, creditorBankIban, creditorBankBic, purchaseAmount, currency, orderResult["payment_description"].(string), iban.Code, orderResult["orderid"].(string))

		log.Infof("[buyHandler] Order accepted: %s from IBAN: %s Amount: %s", orderResult["orderid"].(string), iban.Code, purchaseAmount)
		bot.trySendMessage(m.Sender, fmt.Sprintf("%s", orderConfirmation))
	} else {
		log.Errorln(fmt.Sprintf("[/buyHandler] Error: order not accepted from: %s error: ", lnaddr, orderResult["status"].(string)))
		bot.trySendMessage(m.Sender, fmt.Sprintf("%s", "❌ Order not accepted"))

	}

	return ctx, err
}
