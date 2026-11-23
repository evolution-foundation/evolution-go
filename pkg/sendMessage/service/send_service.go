package send_service

import (
	"bytes"
	"context"
	crypto_rand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/chai2010/webp"
	config "github.com/evolution-foundation/evolution-go/pkg/config"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	logger_wrapper "github.com/evolution-foundation/evolution-go/pkg/logger"
	"github.com/evolution-foundation/evolution-go/pkg/utils"
	whatsmeow_service "github.com/evolution-foundation/evolution-go/pkg/whatsmeow/service"
	"github.com/gabriel-vasile/mimetype"
	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"golang.org/x/net/html"
	"google.golang.org/protobuf/proto"
)

type SendService interface {
	SendText(data *TextStruct, instance *instance_model.Instance) (*MessageSendStruct, error)
	SendLink(data *LinkStruct, instance *instance_model.Instance) (*MessageSendStruct, error)
	SendMediaUrl(data *MediaStruct, instance *instance_model.Instance) (*MessageSendStruct, error)
	SendMediaFile(data *MediaStruct, fileData []byte, instance *instance_model.Instance) (*MessageSendStruct, error)
	SendPoll(data *PollStruct, instance *instance_model.Instance) (*MessageSendStruct, error)
	SendSticker(data *StickerStruct, instance *instance_model.Instance) (*MessageSendStruct, error)
	SendLocation(data *LocationStruct, instance *instance_model.Instance) (*MessageSendStruct, error)
	SendContact(data *ContactStruct, instance *instance_model.Instance) (*MessageSendStruct, error)
	SendButton(data *ButtonStruct, instance *instance_model.Instance) (*MessageSendStruct, error)
	SendList(data *ListStruct, instance *instance_model.Instance) (*MessageSendStruct, error)
	SendCarousel(data *CarouselStruct, instance *instance_model.Instance) (*MessageSendStruct, error)
	SendStatusText(data *StatusTextStruct, instance *instance_model.Instance) (*MessageSendStruct, error)
	SendStatusMediaUrl(data *StatusMediaStruct, instance *instance_model.Instance) (*MessageSendStruct, error)
	SendStatusMediaFile(data *StatusMediaStruct, fileData []byte, instance *instance_model.Instance) (*MessageSendStruct, error)
}

type sendService struct {
	clientPointer    map[string]*whatsmeow.Client
	whatsmeowService whatsmeow_service.WhatsmeowService
	config           *config.Config
	loggerWrapper    *logger_wrapper.LoggerManager
}

type SendDataStruct struct {
	Id              string
	Number          string
	Delay           int32
	MentionAll      bool
	MentionedJID    []string
	FormatJid       *bool
	Quoted          QuotedStruct
	MediaHandle     string
	AdditionalNodes *[]waBinary.Node
	ForwardingScore *uint32
}

type QuotedStruct struct {
	MessageID   string `json:"messageId"`
	Participant string `json:"participant"`
}

type TextStruct struct {
	Number          string       `json:"number"`
	Text            string       `json:"text"`
	Id              string       `json:"id"`
	Delay           int32        `json:"delay"`
	MentionedJID    []string     `json:"mentionedJid"`
	MentionAll      bool         `json:"mentionAll"`
	FormatJid       *bool        `json:"formatJid,omitempty"`
	Quoted          QuotedStruct `json:"quoted"`
	ForwardingScore *uint32      `json:"forwardingScore,omitempty"`
}

type LinkStruct struct {
	Number       string       `json:"number"`
	Text         string       `json:"text"`
	Title        string       `json:"title"`
	Url          string       `json:"url"`
	Description  string       `json:"description"`
	ImgUrl       string       `json:"imgUrl"`
	Id           string       `json:"id"`
	Delay        int32        `json:"delay"`
	MentionedJID []string     `json:"mentionedJid"`
	MentionAll   bool         `json:"mentionAll"`
	FormatJid    *bool        `json:"formatJid,omitempty"`
	Quoted       QuotedStruct `json:"quoted"`
}

type MediaStruct struct {
	Number          string       `json:"number"`
	Url             string       `json:"url"`
	Type            string       `json:"type"`
	Caption         string       `json:"caption"`
	Filename        string       `json:"filename"`
	Id              string       `json:"id"`
	Delay           int32        `json:"delay"`
	MentionedJID    []string     `json:"mentionedJid"`
	MentionAll      bool         `json:"mentionAll"`
	FormatJid       *bool        `json:"formatJid,omitempty"`
	Quoted          QuotedStruct `json:"quoted"`
	ForwardingScore *uint32      `json:"forwardingScore,omitempty"`
}

type PollStruct struct {
	Id           string       `json:"id"`
	Number       string       `json:"number"`
	Question     string       `json:"question"`
	MaxAnswer    int          `json:"maxAnswer"`
	Options      []string     `json:"options"`
	Delay        int32        `json:"delay"`
	MentionedJID []string     `json:"mentionedJid"`
	MentionAll   bool         `json:"mentionAll"`
	FormatJid    *bool        `json:"formatJid,omitempty"`
	Quoted       QuotedStruct `json:"quoted"`
}

type StickerStruct struct {
	Number       string       `json:"number"`
	Sticker      string       `json:"sticker"`
	Id           string       `json:"id"`
	Delay        int32        `json:"delay"`
	MentionedJID []string     `json:"mentionedJid"`
	MentionAll   bool         `json:"mentionAll"`
	FormatJid    *bool        `json:"formatJid,omitempty"`
	Quoted       QuotedStruct `json:"quoted"`
}

type LocationStruct struct {
	Number       string       `json:"number"`
	Id           string       `json:"id"`
	Name         string       `json:"name"`
	Latitude     float64      `json:"latitude"`
	Longitude    float64      `json:"longitude"`
	Address      string       `json:"address"`
	Delay        int32        `json:"delay"`
	MentionedJID []string     `json:"mentionedJid"`
	MentionAll   bool         `json:"mentionAll"`
	FormatJid    *bool        `json:"formatJid,omitempty"`
	Quoted       QuotedStruct `json:"quoted"`
}

type ContactStruct struct {
	Number       string            `json:"number"`
	Id           string            `json:"id"`
	Vcard        utils.VCardStruct `json:"vcard"`
	Delay        int32             `json:"delay"`
	MentionedJID []string          `json:"mentionedJid"`
	MentionAll   bool              `json:"mentionAll"`
	FormatJid    *bool             `json:"formatJid,omitempty"`
	Quoted       QuotedStruct      `json:"quoted"`
}

// Button represents a single interactive button for /send/button.
// The `type` field drives which of the other fields are used:
//   - reply: uses `displayText` + `id`
//   - copy:  uses `displayText` + `copyCode`
//   - url:   uses `displayText` + `url`
//   - call:  uses `displayText` + `phoneNumber`
//   - pix:   uses `currency` + `name` + `keyType` + `key` (must be sent alone)
type Button struct {
	// Button kind. One of: reply, copy, url, call, pix.
	Type string `json:"type" enums:"reply,copy,url,call,pix" example:"reply"`
	// Label rendered inside the button (reply / copy / url / call). Ignored for pix.
	DisplayText string `json:"displayText" example:"Quero saber mais"`
	// Callback payload for `reply` or code-to-copy internal id for `copy`.
	Id string `json:"id" example:"btn_info"`
	// Code placed in the clipboard when type=copy.
	CopyCode string `json:"copyCode,omitempty" example:"PROMO2026"`
	// Target URL when type=url.
	URL string `json:"url,omitempty" example:"https://evolutionapi.com"`
	// Destination phone number (E.164) when type=call.
	PhoneNumber string `json:"phoneNumber,omitempty" example:"+5582988898565"`
	// ISO currency code for type=pix (e.g. BRL).
	Currency string `json:"currency,omitempty" example:"BRL"`
	// Merchant display name shown on the Pix sheet.
	Name string `json:"name,omitempty" example:"Minha Loja"`
	// Pix key type. One of: phone, email, cpf, cnpj, random.
	KeyType string `json:"keyType,omitempty" enums:"phone,email,cpf,cnpj,random" example:"cpf"`
	// Pix key value matching the keyType.
	Key string `json:"key,omitempty" example:"12345678900"`
}

// ButtonStruct is the body for POST /send/button.
//
// Server-side validation:
//   - up to 3 `reply` buttons per message;
//   - `reply` cannot be mixed with any other type;
//   - `pix` must be the only button in the message.
//
// WhatsApp Web rendering quirk (NOT enforced by the server):
//   - mixing `reply` with CTA buttons (copy/url/call) makes the message invisible on WhatsApp Web;
//   - safe combinations: only-reply (up to 3) OR grouped CTAs (copy + url + call).
type ButtonStruct struct {
	// Destination phone number.
	Number string `json:"number" example:"5582988898565"`
	// Header title (required).
	Title string `json:"title" example:"Oferta especial"`
	// Body description text (required).
	Description string `json:"description" example:"Confira as condicoes abaixo"`
	// Footer text (required).
	Footer string `json:"footer" example:"Evolution GO"`
	// Buttons array. See combination rules on the parent type description.
	Buttons []Button `json:"buttons"`
	// Typing delay (milliseconds) applied before sending the message.
	Delay int32 `json:"delay,omitempty" example:"1200"`
	// JIDs to mention inside the body text.
	MentionedJID []string `json:"mentionedJid,omitempty"`
	// Mention every participant (groups only).
	MentionAll bool `json:"mentionAll,omitempty"`
	// If false, skips automatic formatting/validation of `number` into a JID.
	FormatJid *bool `json:"formatJid,omitempty"`
	// Quoted (reply-to) context.
	Quoted QuotedStruct `json:"quoted,omitempty"`
	// Optional image URL used as header for reply-only buttons.
	ImageUrl string `json:"imageUrl,omitempty"`
	// Optional video URL used as header for reply-only buttons.
	VideoUrl string `json:"videoUrl,omitempty"`
}

// Row is a selectable item inside a list Section.
type Row struct {
	// Row main label.
	Title string `json:"title" example:"Plano Basico"`
	// Optional secondary line below the title.
	Description string `json:"description,omitempty" example:"R$ 29,90/mes"`
	// Callback payload returned when the user taps the row. Auto-generated if empty.
	RowId string `json:"rowId,omitempty" example:"plan_basic"`
}

// Section groups related Rows under an optional title.
type Section struct {
	// Section heading (optional; rendered as a group separator).
	Title string `json:"title,omitempty" example:"Planos"`
	// Rows inside this section.
	Rows []Row `json:"rows"`
}

// ListStruct is the body for POST /send/list.
//
// Renders as a single-select menu (legacy ListMessage format — compatible with iOS, Android and WhatsApp Web).
type ListStruct struct {
	// Destination phone number.
	Number string `json:"number" example:"5582988898565"`
	// Header title (required).
	Title string `json:"title" example:"Nossos planos"`
	// Body description text (required).
	Description string `json:"description" example:"Escolha o plano ideal para voce"`
	// Label of the button that opens the list. Defaults to "Ver Menu" when empty.
	ButtonText string `json:"buttonText" example:"Abrir cardapio"`
	// Footer text (required).
	FooterText string `json:"footerText" example:"Evolution GO"`
	// Sections with rows. At least one section with one row is required.
	Sections []Section `json:"sections"`
	// Typing delay (milliseconds) applied before sending the message.
	Delay int32 `json:"delay,omitempty" example:"1200"`
	// JIDs to mention inside the body text.
	MentionedJID []string `json:"mentionedJid,omitempty"`
	// Mention every participant (groups only).
	MentionAll bool `json:"mentionAll,omitempty"`
	// If false, skips automatic formatting/validation of `number` into a JID.
	FormatJid *bool `json:"formatJid,omitempty"`
	// Quoted (reply-to) context.
	Quoted QuotedStruct `json:"quoted,omitempty"`
}

// CarouselButtonStruct is a button attached to a single carousel card.
//
// IMPORTANT — this struct is different from `Button` (used in /send/button):
// it has NO dedicated `url` or `phoneNumber` fields. For URL and CALL buttons
// you must put the link / phone number in the `id` field.
//
//   - REPLY (default): uses `displayText` + `id` as callback payload.
//   - URL:   uses `displayText` + `id` (put the URL here).
//   - CALL:  uses `displayText` + `id` (put the phone number here).
//   - COPY:  uses `displayText` + `copyCode`.
//
// PIX buttons are NOT supported inside carousel cards — use /send/button instead.
//
// WhatsApp Web rendering quirk (NOT enforced by the server):
// avoid mixing REPLY with CTA buttons (URL/CALL/COPY) in the same card —
// mixed sets do not render on WhatsApp Web. Prefer only-REPLY or only-CTAs per card.
type CarouselButtonStruct struct {
	// Button kind (case-insensitive). One of: REPLY (default), URL, CALL, COPY.
	Type string `json:"type" enums:"REPLY,URL,CALL,COPY,reply,url,call,copy" example:"REPLY"`
	// Label rendered inside the button.
	DisplayText string `json:"displayText" example:"Quero saber mais"`
	// Context-dependent: REPLY payload, URL target (type=URL) or phone number (type=CALL).
	Id string `json:"id" example:"card1_info"`
	// Code placed in the clipboard when type=COPY.
	CopyCode string `json:"copyCode,omitempty" example:"PROMO2026"`
}

// CarouselCardHeaderStruct is the top area of a carousel card.
// Either `imageUrl` OR `videoUrl` may be provided (image takes precedence when both are set).
type CarouselCardHeaderStruct struct {
	// Optional visible title above the media.
	Title string `json:"title,omitempty" example:"Oferta do dia"`
	// Optional subtitle rendered below the title.
	Subtitle string `json:"subtitle,omitempty" example:"Somente hoje"`
	// Public URL to an image. Downloaded, uploaded to WhatsApp servers and used as card media.
	ImageUrl string `json:"imageUrl,omitempty" example:"https://picsum.photos/seed/card1/600/400"`
	// Public URL to a video. Used only when `imageUrl` is empty.
	VideoUrl string `json:"videoUrl,omitempty"`
}

// CarouselCardBodyStruct is the text area of a carousel card.
type CarouselCardBodyStruct struct {
	// Main text of the card.
	Text string `json:"text" example:"Card 1 - Oferta especial"`
}

// CarouselCardStruct is a single card inside a carousel message.
// Each card requires at least `header` + `body`.
type CarouselCardStruct struct {
	// Card header (media + title/subtitle).
	Header CarouselCardHeaderStruct `json:"header"`
	// Card body text (required).
	Body CarouselCardBodyStruct `json:"body"`
	// Optional footer rendered under the body.
	Footer string `json:"footer,omitempty" example:"Por tempo limitado"`
	// Buttons shown on the card. See CarouselButtonStruct for combination rules.
	Buttons []CarouselButtonStruct `json:"buttons,omitempty"`
}

// CarouselStruct is the body for POST /send/carousel.
//
// Sends an interactive carousel of swipeable cards. At least one card is required.
// Each card must have `header` + `body`; button rules are described on CarouselButtonStruct.
type CarouselStruct struct {
	// Destination phone number.
	Number string `json:"number" example:"5582988898565"`
	// Optional message body shown above the cards.
	Body string `json:"body,omitempty" example:"Confira nossas novidades!"`
	// Optional message footer shown below the cards.
	Footer string `json:"footer,omitempty" example:"Evolution GO"`
	// Typing delay (milliseconds) applied before sending the message.
	Delay int32 `json:"delay,omitempty" example:"1200"`
	// If false, skips automatic formatting/validation of `number` into a JID.
	FormatJid *bool `json:"formatJid,omitempty"`
	// Quoted (reply-to) context.
	Quoted QuotedStruct `json:"quoted,omitempty"`
	// Cards displayed in order. At least one card is required.
	Cards []CarouselCardStruct `json:"cards"`
}

type StatusTextStruct struct {
	Text string `json:"text"`
	Id   string `json:"id"`
}

type StatusMediaStruct struct {
	Type    string `json:"type"`
	Url     string `json:"url"`
	Caption string `json:"caption"`
	Id      string `json:"id"`
}

type MessageSendStruct struct {
	Info               types.MessageInfo
	Message            *waE2E.Message
	MessageContextInfo *waE2E.ContextInfo
}

func (s *sendService) ensureClientConnected(instanceId string) (*whatsmeow.Client, error) {
	client := s.clientPointer[instanceId]
	s.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Checking client connection status - Client exists: %v", instanceId, client != nil)

	if client == nil {
		s.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] No client found, attempting to start new instance", instanceId)
		err := s.whatsmeowService.StartInstance(instanceId)
		if err != nil {
			s.loggerWrapper.GetLogger(instanceId).LogError("[%s] Failed to start instance: %v", instanceId, err)
			return nil, errors.New("no active session found")
		}

		s.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Instance started, waiting 2 seconds...", instanceId)
		time.Sleep(2 * time.Second)

		client = s.clientPointer[instanceId]
		s.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Checking new client - Exists: %v, Connected: %v",
			instanceId,
			client != nil,
			client != nil && client.IsConnected())

		if client == nil || !client.IsConnected() {
			s.loggerWrapper.GetLogger(instanceId).LogError("[%s] New client validation failed - Exists: %v, Connected: %v",
				instanceId,
				client != nil,
				client != nil && client.IsConnected())
			return nil, errors.New("no active session found")
		}
	} else if !client.IsConnected() {
		s.loggerWrapper.GetLogger(instanceId).LogError("[%s] Existing client is disconnected - Connected status: %v",
			instanceId,
			client.IsConnected())
		return nil, errors.New("client disconnected")
	}

	s.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Client successfully validated - Connected: %v", instanceId, client.IsConnected())
	return client, nil
}

// ensureClientConnectedWithRetry attempts to ensure client connection with automatic reconnection and retry
func (s *sendService) ensureClientConnectedWithRetry(instanceId string, maxRetries int) (*whatsmeow.Client, error) {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		s.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Connection attempt %d/%d", instanceId, attempt, maxRetries)

		client, err := s.ensureClientConnected(instanceId)
		if err == nil {
			return client, nil
		}

		// Check if it's a disconnection error that we can retry
		if err.Error() == "client disconnected" || err.Error() == "no active session found" {
			s.loggerWrapper.GetLogger(instanceId).LogWarn("[%s] Client disconnected on attempt %d/%d, attempting reconnection...", instanceId, attempt, maxRetries)

			// Attempt to reconnect the client
			reconnectErr := s.whatsmeowService.ReconnectClient(instanceId)
			if reconnectErr != nil {
				s.loggerWrapper.GetLogger(instanceId).LogError("[%s] Failed to reconnect client on attempt %d: %v", instanceId, attempt, reconnectErr)
			} else {
				s.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Reconnection initiated on attempt %d, waiting 3 seconds...", instanceId, attempt)
				time.Sleep(3 * time.Second)
			}

			// If this is not the last attempt, continue to retry
			if attempt < maxRetries {
				waitTime := time.Duration(attempt*2) * time.Second // Progressive backoff
				s.loggerWrapper.GetLogger(instanceId).LogInfo("[%s] Waiting %v before retry attempt %d", instanceId, waitTime, attempt+1)
				time.Sleep(waitTime)
				continue
			}
		}

		// If it's the last attempt or a non-retryable error, return the error
		s.loggerWrapper.GetLogger(instanceId).LogError("[%s] Failed to ensure client connection after %d attempts: %v", instanceId, attempt, err)
		return nil, err
	}

	return nil, fmt.Errorf("failed to connect client after %d attempts", maxRetries)
}

func validateMessageFields(phone string, formatJid *bool, messageID *string, participant *string) (types.JID, error) {
	// Apply formatting if formatJid is true (default)
	shouldFormat := true // Default value
	if formatJid != nil {
		shouldFormat = *formatJid
	}

	var finalPhone string
	if shouldFormat {
		// Extract raw number if it's already a JID, then apply CreateJID formatting
		rawNumber := phone
		if strings.Contains(phone, "@s.whatsapp.net") {
			rawNumber = strings.Split(phone, "@")[0]
		}

		normalizedJID, err := utils.CreateJID(rawNumber)
		if err != nil {
			// If CreateJID fails, try with ParseJID as fallback
			recipient, ok := utils.ParseJID(phone)
			if !ok {
				return types.NewJID("", types.DefaultUserServer), fmt.Errorf("could not parse phone: %s", phone)
			}
			finalPhone = recipient.String()
		} else {
			finalPhone = normalizedJID
		}
	} else {
		// Use phone as received without formatting
		finalPhone = phone
	}

	recipient, ok := utils.ParseJID(finalPhone)
	if !ok {
		return types.NewJID("", types.DefaultUserServer), errors.New("could not parse formatted phone")
	}

	if messageID != nil {
		if participant == nil {
			return types.NewJID("", types.DefaultUserServer), errors.New("missing Participant in ContextInfo")
		}
	}

	if participant != nil {
		if messageID == nil {
			return types.NewJID("", types.DefaultUserServer), errors.New("missing StanzaId in ContextInfo")
		}
	}

	return recipient, nil
}

// validateAndCheckUserExists validates message fields and checks if the user exists on WhatsApp
// Now uses the new approach: CheckUser with formatJid=false by default, and uses remoteJID for messaging
func (s *sendService) validateAndCheckUserExists(phone string, formatJid *bool, messageID *string, participant *string, instance *instance_model.Instance) (types.JID, error) {
	// Skip WhatsApp check if disabled in config
	if !s.config.CheckUserExists {
		s.loggerWrapper.GetLogger(instance.Id).LogDebug("[%s] User existence check disabled by configuration", instance.Id)
		// Use original validation logic when check is disabled
		return validateMessageFields(phone, formatJid, messageID, participant)
	}

	// Skip WhatsApp check for group messages, broadcast, newsletter, and LID
	if strings.Contains(phone, "@g.us") || strings.Contains(phone, "@broadcast") || strings.Contains(phone, "@newsletter") || strings.Contains(phone, "@lid") {
		return validateMessageFields(phone, formatJid, messageID, participant)
	}

	// Get the client to check if user exists on WhatsApp
	client, err := s.ensureClientConnected(instance.Id)
	if err != nil {
		return types.NewJID("", types.DefaultUserServer), fmt.Errorf("failed to connect client: %v", err)
	}

	// Use CheckUser approach: formatJid=false by default
	formatJidForCheck := false

	// First attempt with formatJid=false
	remoteJID, found, err := s.checkSingleUserExists(client, phone, formatJidForCheck, instance.Id)
	if err != nil {
		s.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] Failed to check user existence: %v", instance.Id, err)
		// Continue with sending even if check fails (network issues, etc.)
		return validateMessageFields(phone, formatJid, messageID, participant)
	}

	// If not found with formatJid=false, try with formatJid=true as fallback
	if !found {
		s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] User not found with formatJid=false, trying with formatJid=true", instance.Id)
		remoteJIDRetry, foundRetry, errRetry := s.checkSingleUserExists(client, phone, true, instance.Id)
		if errRetry == nil && foundRetry {
			remoteJID = remoteJIDRetry
			found = foundRetry
		}
	}

	if !found {
		return types.NewJID("", types.DefaultUserServer), fmt.Errorf("number %s is not registered on WhatsApp", phone)
	}

	s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Number %s verified as valid WhatsApp user, using remoteJID: %s", instance.Id, phone, remoteJID)

	// Validate the remoteJID with formatJid=false for message sending
	formatJidFalse := false
	return validateMessageFields(remoteJID, &formatJidFalse, messageID, participant)
}

// checkSingleUserExists checks if a single user exists on WhatsApp with the specified formatJid setting
// Returns: remoteJID, found, error
func (s *sendService) checkSingleUserExists(client *whatsmeow.Client, phone string, formatJid bool, instanceId string) (string, bool, error) {
	phoneNumbers, err := utils.PrepareNumbersForWhatsAppCheck([]string{phone}, &formatJid)
	if err != nil {
		return "", false, fmt.Errorf("failed to prepare number for WhatsApp check: %v", err)
	}

	// Check if the number exists on WhatsApp
	resp, err := client.IsOnWhatsApp(context.Background(), phoneNumbers)
	if err != nil {
		return "", false, fmt.Errorf("failed to check if number %s exists on WhatsApp: %v", phoneNumbers[0], err)
	}

	// Verify if the number was found
	if len(resp) == 0 {
		return "", false, fmt.Errorf("number %s not found in WhatsApp response", phoneNumbers[0])
	}

	// Check if the first result indicates the number is on WhatsApp
	if !resp[0].IsIn {
		return "", false, nil // Not an error, just not found
	}

	// Return the remoteJID from WhatsApp's response
	remoteJID := fmt.Sprintf("%v", resp[0].JID)
	return remoteJID, true, nil
}

func findURL(text string) string {
	urlRegex := `http[s]?://(?:[a-zA-Z]|[0-9]|[$-_@.&+]|[!*\\(\\),]|(?:%[0-9a-fA-F][0-9a-fA-F]))+`
	re := regexp.MustCompile(urlRegex)
	urls := re.FindAllString(text, -1)
	if len(urls) > 0 {
		return urls[0]
	}
	return ""
}

func (s *sendService) SendText(data *TextStruct, instance *instance_model.Instance) (*MessageSendStruct, error) {
	return s.sendTextWithRetry(data, instance, 3) // 3 tentativas máximas
}

func (s *sendService) sendTextWithRetry(data *TextStruct, instance *instance_model.Instance, maxRetries int) (*MessageSendStruct, error) {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] SendText attempt %d/%d", instance.Id, attempt, maxRetries)

		_, err := s.ensureClientConnectedWithRetry(instance.Id, 2)
		if err != nil {
			if attempt == maxRetries {
				return nil, err
			}
			continue
		}

		msg := &waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text: &data.Text,
			},
		}

		message, err := s.SendMessage(instance, msg, "ExtendedTextMessage", &SendDataStruct{
			Id:              data.Id,
			Number:          data.Number,
			Quoted:          data.Quoted,
			Delay:           data.Delay,
			MentionAll:      data.MentionAll,
			MentionedJID:    data.MentionedJID,
			FormatJid:       data.FormatJid,
			ForwardingScore: data.ForwardingScore,
		})

		if err != nil {
			// Check if it's a client disconnection error
			if strings.Contains(err.Error(), "client disconnected") || strings.Contains(err.Error(), "no active session") {
				s.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] SendText failed due to disconnection on attempt %d/%d: %v", instance.Id, attempt, maxRetries, err)
				if attempt < maxRetries {
					waitTime := time.Duration(attempt) * time.Second
					s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Waiting %v before retry", instance.Id, waitTime)
					time.Sleep(waitTime)
					continue
				}
			}
			return nil, err
		}

		s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] SendText successful on attempt %d", instance.Id, attempt)
		return message, nil
	}

	return nil, fmt.Errorf("failed to send text after %d attempts", maxRetries)
}

func fetchLinkMetadata(url string) (string, string, string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", "", "", err
	}
	defer resp.Body.Close()

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return "", "", "", err
	}

	var title, description, imgURL string

	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if n.Data == "title" && n.FirstChild != nil {
				title = n.FirstChild.Data
			}
			if n.Data == "meta" {
				var property, content string
				for _, attr := range n.Attr {
					if attr.Key == "property" || attr.Key == "name" {
						property = attr.Val
					}
					if attr.Key == "content" {
						content = attr.Val
					}
				}

				if (property == "description" || property == "og:description") && content != "" {
					description = content
				}

				if property == "og:image" && content != "" {
					imgURL = content
				}
			}
		}

		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}

	f(doc)

	return title, description, imgURL, nil
}

func (s *sendService) SendLink(data *LinkStruct, instance *instance_model.Instance) (*MessageSendStruct, error) {
	return s.sendLinkWithRetry(data, instance, 3)
}

func (s *sendService) sendLinkWithRetry(data *LinkStruct, instance *instance_model.Instance, maxRetries int) (*MessageSendStruct, error) {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] SendLink attempt %d/%d", instance.Id, attempt, maxRetries)

		_, err := s.ensureClientConnectedWithRetry(instance.Id, 2)
		if err != nil {
			if attempt == maxRetries {
				return nil, err
			}
			continue
		}

		matchedText := findURL(data.Text)

		if matchedText != "" {
			title, description, imgUrl, err := fetchLinkMetadata(matchedText)
			if err != nil {
				if attempt == maxRetries {
					return nil, err
				}
				continue
			}

			data.Title = title
			data.Description = description
			data.ImgUrl = imgUrl
		}

		var fileData []byte
		if data.ImgUrl != "" {
			resp, err := http.Get(data.ImgUrl)
			if err != nil {
				if attempt == maxRetries {
					return nil, err
				}
				continue
			}
			defer resp.Body.Close()
			fileData, _ = io.ReadAll(resp.Body)
		}

		previewType := waE2E.ExtendedTextMessage_VIDEO
		msg := &waE2E.Message{
			ExtendedTextMessage: &waE2E.ExtendedTextMessage{
				Text:          &data.Text,
				Title:         &data.Title,
				MatchedText:   &matchedText,
				JPEGThumbnail: fileData,
				Description:   &data.Description,
				PreviewType:   &previewType,
			},
		}

		message, err := s.SendMessage(instance, msg, "ExtendedTextMessage", &SendDataStruct{
			Id:           data.Id,
			Number:       data.Number,
			Quoted:       data.Quoted,
			Delay:        data.Delay,
			MentionAll:   data.MentionAll,
			MentionedJID: data.MentionedJID,
			FormatJid:    data.FormatJid,
		})

		if err != nil {
			// Check if it's a client disconnection error
			if strings.Contains(err.Error(), "client disconnected") || strings.Contains(err.Error(), "no active session") {
				s.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] SendLink failed due to disconnection on attempt %d/%d: %v", instance.Id, attempt, maxRetries, err)
				if attempt < maxRetries {
					waitTime := time.Duration(attempt) * time.Second
					s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Waiting %v before retry", instance.Id, waitTime)
					time.Sleep(waitTime)
					continue
				}
			}
			return nil, err
		}

		s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] SendLink successful on attempt %d", instance.Id, attempt)
		return message, nil
	}

	return nil, fmt.Errorf("failed to send link after %d attempts", maxRetries)
}

type ConvertAudio struct {
	Url    string `json:"url,omitempty"`
	Base64 string `json:"base64,omitempty"`
}

type ApiResponse struct {
	Duration int    `json:"duration"`
	Audio    string `json:"audio"`
}

func convertAudioWithApi(apiUrl string, apiKey string, convertData ConvertAudio) ([]byte, int, error) {
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	// Adiciona o campo "url" ao form-data se a URL for fornecida
	if convertData.Url != "" {
		err := writer.WriteField("url", convertData.Url)
		if err != nil {
			return nil, 0, fmt.Errorf("erro ao adicionar a URL no form-data: %v", err)
		}
	}

	// Adiciona o campo "base64" ao form-data se a string base64 for fornecida
	if convertData.Base64 != "" {
		err := writer.WriteField("base64", convertData.Base64)
		if err != nil {
			return nil, 0, fmt.Errorf("erro ao adicionar o base64 no form-data: %v", err)
		}
	}

	// Fecha o writer multipart
	err := writer.Close()
	if err != nil {
		return nil, 0, fmt.Errorf("erro ao finalizar o form-data: %v", err)
	}

	req, err := http.NewRequest("POST", apiUrl, &requestBody)
	if err != nil {
		return nil, 0, fmt.Errorf("erro ao criar a requisição: %v", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("apikey", apiKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("erro ao enviar a requisição: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("erro ao ler a resposta: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("requisição falhou com status: %d, resposta: %s", resp.StatusCode, string(body))
	}

	var apiResponse ApiResponse
	err = json.Unmarshal(body, &apiResponse)
	if err != nil {
		return nil, 0, fmt.Errorf("erro ao deserializar a resposta: %v", err)
	}

	base64ToBytes, err := base64.StdEncoding.DecodeString(apiResponse.Audio)
	if err != nil {
		return nil, 0, fmt.Errorf("erro ao decodificar o áudio: %v", err)
	}

	return base64ToBytes, apiResponse.Duration, nil
}

func convertAudioToOpusWithDuration(inputData []byte) ([]byte, int, error) {
	cmd := exec.Command("ffmpeg", "-i", "pipe:0",
		"-f",
		"ogg",
		"-vn",
		"-c:a",
		"libopus",
		"-avoid_negative_ts",
		"make_zero",
		"-b:a",
		"128k",
		"-ar",
		"48000",
		"-ac",
		"1",
		"-write_xing",
		"0",
		"-compression_level",
		"10",
		"-application",
		"voip",
		"-fflags",
		"+bitexact",
		"-flags",
		"+bitexact",
		"-id3v2_version",
		"0",
		"-map_metadata",
		"-1",
		"-map_chapters",
		"-1",
		"-write_bext",
		"0",
		"pipe:1",
	)

	var outBuffer bytes.Buffer
	var errBuffer bytes.Buffer

	cmd.Stdin = bytes.NewReader(inputData)
	cmd.Stdout = &outBuffer
	cmd.Stderr = &errBuffer

	err := cmd.Run()
	if err != nil {
		return nil, 0, fmt.Errorf("error during conversion: %v, details: %s", err, errBuffer.String())
	}

	convertedData := outBuffer.Bytes()

	outputText := errBuffer.String()

	splitTime := strings.Split(outputText, "time=")

	if len(splitTime) < 2 {
		return nil, 0, errors.New("duração não encontrada")
	}

	// Use the last occurrence of time= in case there are multiple
	timeString := splitTime[len(splitTime)-1]

	re := regexp.MustCompile(`(\d+):(\d+):(\d+\.\d+)`)
	matches := re.FindStringSubmatch(timeString)
	if len(matches) != 4 {
		return nil, 0, errors.New("formato de duração não encontrado")
	}

	hours, _ := strconv.ParseFloat(matches[1], 64)
	minutes, _ := strconv.ParseFloat(matches[2], 64)
	seconds, _ := strconv.ParseFloat(matches[3], 64)
	duration := int(hours*3600 + minutes*60 + seconds)

	return convertedData, duration, nil
}

func (s *sendService) SendMediaFile(data *MediaStruct, fileData []byte, instance *instance_model.Instance) (*MessageSendStruct, error) {
	return s.sendMediaFileWithRetry(data, fileData, instance, 3)
}

func (s *sendService) sendMediaFileWithRetry(data *MediaStruct, fileData []byte, instance *instance_model.Instance, maxRetries int) (*MessageSendStruct, error) {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] SendMediaFile attempt %d/%d", instance.Id, attempt, maxRetries)

		client, err := s.ensureClientConnectedWithRetry(instance.Id, 2)
		if err != nil {
			if attempt == maxRetries {
				return nil, err
			}
			continue
		}

		mime, _ := mimetype.DetectReader(bytes.NewReader(fileData))
		mimeType := mime.String()

		var uploadType whatsmeow.MediaType
		var duration int

		switch data.Type {
		case "image":
			if mimeType != "image/jpeg" && mimeType != "image/png" && mimeType != "image/webp" {
				errMsg := fmt.Sprintf("Invalid file format: '%s'. Only 'image/jpeg', 'image/png' and 'image/webp' are accepted", mimeType)
				return nil, errors.New(errMsg)
			}
			if mimeType == "image/webp" {
				mimeType = "image/jpeg"
			}
			uploadType = whatsmeow.MediaImage
		case "video":
			if mimeType != "video/mp4" {
				errMsg := fmt.Sprintf("Invalid file format: '%s'. Only 'video/mp4' is accepted", mimeType)
				return nil, errors.New(errMsg)
			}
			uploadType = whatsmeow.MediaVideo
		case "audio":
			converterApiUrl := s.config.ApiAudioConverter
			converterApiKey := s.config.ApiAudioConverterKey
			var convertedData []byte
			var err error
			if converterApiUrl == "" {

				convertedData, duration, err = convertAudioToOpusWithDuration(fileData)
				if err != nil {
					return nil, err
				}
			} else {
				convertedData, duration, err = convertAudioWithApi(converterApiUrl, converterApiKey, ConvertAudio{Base64: base64.StdEncoding.EncodeToString(fileData)})
				if err != nil {
					return nil, err
				}
			}

			fileData = convertedData
			mimeType = "audio/ogg; codecs=opus"
			uploadType = whatsmeow.MediaAudio
		case "document":
			uploadType = whatsmeow.MediaDocument
		default:
			return nil, errors.New("invalid media type")
		}

		// Detectar se é newsletter para usar upload sem criptografia
		isNewsletter := strings.Contains(data.Number, "@newsletter")

		// Validar se é documento em newsletter (não suportado)
		if isNewsletter && data.Type == "document" {
			return nil, errors.New("documentos não são suportados em canais do WhatsApp. Use imagem, vídeo, áudio ou enquete")
		}

		s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] SendMediaFile - Upload iniciado (Newsletter: %v)...", instance.Id, isNewsletter)

		var uploaded whatsmeow.UploadResponse
		if isNewsletter {
			// Newsletter: upload SEM criptografia
			uploaded, err = client.UploadNewsletter(context.Background(), fileData, uploadType)
			s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Newsletter upload - Handle: %s", instance.Id, uploaded.Handle)
		} else {
			// Normal: upload COM criptografia
			uploaded, err = client.Upload(context.Background(), fileData, uploadType)
		}

		if err != nil {
			return nil, err
		}

		s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Media uploaded with size %d", instance.Id, uploaded.FileLength)

		var media *waE2E.Message
		var mediaType string

		switch data.Type {
		case "image":
			// Generate a JPEG preview thumbnail for better client UX (iOS in
			// particular). On failure jpegThumb is nil and the message is sent
			// without a preview rather than failing the request.
			jpegThumb := makeJPEGThumbnail(fileData, 72)
			if isNewsletter {
				// Newsletter: SEM MediaKey e FileEncSHA256
				media = &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
					Caption:       proto.String(data.Caption),
					URL:           &uploaded.URL,
					DirectPath:    &uploaded.DirectPath,
					Mimetype:      proto.String(mimeType),
					FileSHA256:    uploaded.FileSHA256,
					FileLength:    &uploaded.FileLength,
					JPEGThumbnail: jpegThumb,
				}}
			} else {
				// Normal: COM MediaKey e FileEncSHA256
				media = &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
					Caption:       proto.String(data.Caption),
					URL:           proto.String(uploaded.URL),
					DirectPath:    proto.String(uploaded.DirectPath),
					MediaKey:      uploaded.MediaKey,
					Mimetype:      proto.String(mimeType),
					FileEncSHA256: uploaded.FileEncSHA256,
					FileSHA256:    uploaded.FileSHA256,
					FileLength:    proto.Uint64(uint64(len(fileData))),
					JPEGThumbnail: jpegThumb,
				}}
			}
			mediaType = "ImageMessage"
		case "video":
			if isNewsletter {
				media = &waE2E.Message{VideoMessage: &waE2E.VideoMessage{
					Caption:    proto.String(data.Caption),
					URL:        &uploaded.URL,
					DirectPath: &uploaded.DirectPath,
					Mimetype:   proto.String(mimeType),
					FileSHA256: uploaded.FileSHA256,
					FileLength: &uploaded.FileLength,
				}}
			} else {
				media = &waE2E.Message{VideoMessage: &waE2E.VideoMessage{
					Caption:       proto.String(data.Caption),
					URL:           proto.String(uploaded.URL),
					DirectPath:    proto.String(uploaded.DirectPath),
					MediaKey:      uploaded.MediaKey,
					Mimetype:      proto.String(mimeType),
					FileEncSHA256: uploaded.FileEncSHA256,
					FileSHA256:    uploaded.FileSHA256,
					FileLength:    proto.Uint64(uint64(len(fileData))),
				}}
			}
			mediaType = "VideoMessage"
		case "ptv":
			if isNewsletter {
				media = &waE2E.Message{PtvMessage: &waE2E.VideoMessage{
					URL:        &uploaded.URL,
					DirectPath: &uploaded.DirectPath,
					Mimetype:   proto.String(mimeType),
					FileSHA256: uploaded.FileSHA256,
					FileLength: &uploaded.FileLength,
				}}
			} else {
				media = &waE2E.Message{PtvMessage: &waE2E.VideoMessage{
					URL:           proto.String(uploaded.URL),
					DirectPath:    proto.String(uploaded.DirectPath),
					MediaKey:      uploaded.MediaKey,
					Mimetype:      proto.String(mimeType),
					FileEncSHA256: uploaded.FileEncSHA256,
					FileSHA256:    uploaded.FileSHA256,
					FileLength:    proto.Uint64(uint64(len(fileData))),
				}}
			}
			mediaType = "PtvMessage"
		case "audio":
			if isNewsletter {
				media = &waE2E.Message{AudioMessage: &waE2E.AudioMessage{
					URL:        &uploaded.URL,
					PTT:        proto.Bool(true),
					DirectPath: &uploaded.DirectPath,
					Mimetype:   proto.String(mimeType),
					FileSHA256: uploaded.FileSHA256,
					FileLength: &uploaded.FileLength,
					Seconds:    proto.Uint32(uint32(duration)),
				}}
			} else {
				media = &waE2E.Message{AudioMessage: &waE2E.AudioMessage{
					URL:           proto.String(uploaded.URL),
					PTT:           proto.Bool(true),
					DirectPath:    proto.String(uploaded.DirectPath),
					MediaKey:      uploaded.MediaKey,
					Mimetype:      proto.String(mimeType),
					FileEncSHA256: uploaded.FileEncSHA256,
					FileSHA256:    uploaded.FileSHA256,
					FileLength:    proto.Uint64(uploaded.FileLength),
					Seconds:       proto.Uint32(uint32(duration)),
				}}
			}
			mediaType = "AudioMessage"
		case "document":
			// For PDF documents, rasterize page 1 into a JPEG preview thumbnail.
			// A missing pdftoppm or a failure yields nil and the document is
			// sent without a preview instead of failing the request.
			var jpegThumb []byte
			if mimeType == "application/pdf" {
				jpegThumb = makePDFThumbnail(fileData, 200)
			}
			if isNewsletter {
				media = &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{
					FileName:      &data.Filename,
					Caption:       proto.String(data.Caption),
					URL:           &uploaded.URL,
					DirectPath:    &uploaded.DirectPath,
					Mimetype:      proto.String(mimeType),
					FileSHA256:    uploaded.FileSHA256,
					FileLength:    &uploaded.FileLength,
					JPEGThumbnail: jpegThumb,
				}}
			} else {
				media = &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{
					FileName:      &data.Filename,
					Caption:       proto.String(data.Caption),
					URL:           proto.String(uploaded.URL),
					DirectPath:    proto.String(uploaded.DirectPath),
					MediaKey:      uploaded.MediaKey,
					Mimetype:      proto.String(mimeType),
					FileEncSHA256: uploaded.FileEncSHA256,
					FileSHA256:    uploaded.FileSHA256,
					FileLength:    proto.Uint64(uint64(len(fileData))),
					JPEGThumbnail: jpegThumb,
				}}
			}

			if media.GetDocumentMessage().GetCaption() != "" {
				media.DocumentWithCaptionMessage = &waE2E.FutureProofMessage{
					Message: &waE2E.Message{
						DocumentMessage: media.DocumentMessage,
					},
				}
				media.DocumentMessage = nil
			}

			mediaType = "DocumentMessage"
		default:
			return nil, errors.New("invalid media type")
		}

		message, err := s.SendMessage(instance, media, mediaType, &SendDataStruct{
			Id:              data.Id,
			Number:          data.Number,
			Quoted:          data.Quoted,
			Delay:           data.Delay,
			MentionAll:      data.MentionAll,
			MentionedJID:    data.MentionedJID,
			FormatJid:       data.FormatJid,
			MediaHandle:     uploaded.Handle,
			ForwardingScore: data.ForwardingScore,
		})

		if err != nil {
			// Check if it's a client disconnection error
			if strings.Contains(err.Error(), "client disconnected") || strings.Contains(err.Error(), "no active session") {
				s.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] SendMediaFile failed due to disconnection on attempt %d/%d: %v", instance.Id, attempt, maxRetries, err)
				if attempt < maxRetries {
					waitTime := time.Duration(attempt) * time.Second
					s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Waiting %v before retry", instance.Id, waitTime)
					time.Sleep(waitTime)
					continue
				}
			}
			return nil, err
		}

		s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] SendMediaFile successful on attempt %d", instance.Id, attempt)
		return message, nil
	}

	return nil, fmt.Errorf("failed to send media file after %d attempts", maxRetries)
}

func (s *sendService) SendMediaUrl(data *MediaStruct, instance *instance_model.Instance) (*MessageSendStruct, error) {
	return s.sendMediaUrlWithRetry(data, instance, 3)
}

func (s *sendService) sendMediaUrlWithRetry(data *MediaStruct, instance *instance_model.Instance, maxRetries int) (*MessageSendStruct, error) {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] SendMediaUrl attempt %d/%d for URL: %s", instance.Id, attempt, maxRetries, data.Url)
		startTime := time.Now()

		client, err := s.ensureClientConnectedWithRetry(instance.Id, 2)
		if err != nil {
			if attempt == maxRetries {
				return nil, err
			}
			continue
		}

		s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Iniciando download da URL: %s", instance.Id, data.Url)

		resp, err := http.Get(data.Url)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Download concluído em %v. Lendo dados...", instance.Id, time.Since(startTime))

		downloadStart := time.Now()
		fileData, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Leitura dos dados concluída em %v. Tamanho: %d bytes", instance.Id, time.Since(downloadStart), len(fileData))

		mime, _ := mimetype.DetectReader(bytes.NewReader(fileData))
		mimeType := mime.String()
		if strings.HasSuffix(strings.ToLower(data.Url), ".mp4") {
			mimeType = "video/mp4"
		}

		s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Tipo MIME detectado: %s", instance.Id, mimeType)

		var uploadType whatsmeow.MediaType
		var duration int

		processingStart := time.Now()
		switch data.Type {
		case "image":
			if mimeType != "image/jpeg" && mimeType != "image/png" && mimeType != "image/webp" {
				errMsg := fmt.Sprintf("Invalid file format: '%s'. Only 'image/jpeg', 'image/png' and 'image/webp' are accepted", mimeType)
				return nil, errors.New(errMsg)
			}
			if mimeType == "image/webp" {
				mimeType = "image/jpeg"
			}
			uploadType = whatsmeow.MediaImage

		case "video", "ptv":
			if mimeType != "video/mp4" {
				errMsg := fmt.Sprintf("Invalid file format: '%s'. Only 'video/mp4' are accepted", mimeType)
				return nil, errors.New(errMsg)
			}
			uploadType = whatsmeow.MediaVideo
		case "audio":
			s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Iniciando conversão de áudio...", instance.Id)
			converterApiUrl := s.config.ApiAudioConverter
			converterApiKey := s.config.ApiAudioConverterKey
			var convertedData []byte
			var err error
			if converterApiUrl == "" {
				s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Usando conversão local...", instance.Id)
				convertedData, duration, err = convertAudioToOpusWithDuration(fileData)
			} else {
				s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Usando API de conversão...", instance.Id)
				convertedData, duration, err = convertAudioWithApi(converterApiUrl, converterApiKey, ConvertAudio{Base64: base64.StdEncoding.EncodeToString(fileData)})
			}
			if err != nil {
				return nil, err
			}
			fileData = convertedData
			mimeType = "audio/ogg; codecs=opus"
			uploadType = whatsmeow.MediaAudio
			s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Conversão de áudio concluída em %v", instance.Id, time.Since(processingStart))
		case "document":
			uploadType = whatsmeow.MediaDocument
		default:
			return nil, errors.New("invalid media type")
		}

		// Detectar se é newsletter para usar upload sem criptografia
		isNewsletter := strings.Contains(data.Number, "@newsletter")

		// Validar se é documento em newsletter (não suportado)
		if isNewsletter && data.Type == "document" {
			return nil, errors.New("documentos não são suportados em canais do WhatsApp. Use imagem, vídeo, áudio ou enquete")
		}

		s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Iniciando upload para WhatsApp (Newsletter: %v)...", instance.Id, isNewsletter)
		uploadStart := time.Now()

		var uploaded whatsmeow.UploadResponse
		if isNewsletter {
			// Newsletter: upload sem criptografia
			uploaded, err = client.UploadNewsletter(context.Background(), fileData, uploadType)
			s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Newsletter upload - Handle: %s", instance.Id, uploaded.Handle)
		} else {
			// Upload normal com criptografia
			uploaded, err = client.Upload(context.Background(), fileData, uploadType)
		}

		if err != nil {
			return nil, err
		}
		s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Upload concluído em %v. Tamanho: %d", instance.Id, time.Since(uploadStart), uploaded.FileLength)

		var media *waE2E.Message
		var mediaType string

		switch data.Type {
		case "image":
			// Generate a JPEG preview thumbnail for better client UX (iOS in
			// particular). On failure jpegThumb is nil and the message is sent
			// without a preview rather than failing the request.
			jpegThumb := makeJPEGThumbnail(fileData, 72)
			if isNewsletter {
				// Newsletter: sem criptografia (sem MediaKey e FileEncSHA256)
				media = &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
					Caption:       proto.String(data.Caption),
					URL:           &uploaded.URL,
					DirectPath:    &uploaded.DirectPath,
					Mimetype:      proto.String(mimeType),
					FileSHA256:    uploaded.FileSHA256,
					FileLength:    &uploaded.FileLength,
					JPEGThumbnail: jpegThumb,
				}}
			} else {
				// Normal: com criptografia
				media = &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
					Caption:       proto.String(data.Caption),
					URL:           proto.String(uploaded.URL),
					DirectPath:    proto.String(uploaded.DirectPath),
					MediaKey:      uploaded.MediaKey,
					Mimetype:      proto.String(mimeType),
					FileEncSHA256: uploaded.FileEncSHA256,
					FileSHA256:    uploaded.FileSHA256,
					FileLength:    proto.Uint64(uint64(len(fileData))),
					JPEGThumbnail: jpegThumb,
				}}
			}
			mediaType = "ImageMessage"
		case "video":
			if isNewsletter {
				media = &waE2E.Message{VideoMessage: &waE2E.VideoMessage{
					Caption:    proto.String(data.Caption),
					URL:        &uploaded.URL,
					DirectPath: &uploaded.DirectPath,
					Mimetype:   proto.String(mimeType),
					FileSHA256: uploaded.FileSHA256,
					FileLength: &uploaded.FileLength,
				}}
			} else {
				media = &waE2E.Message{VideoMessage: &waE2E.VideoMessage{
					Caption:       proto.String(data.Caption),
					URL:           proto.String(uploaded.URL),
					DirectPath:    proto.String(uploaded.DirectPath),
					MediaKey:      uploaded.MediaKey,
					Mimetype:      proto.String(mimeType),
					FileEncSHA256: uploaded.FileEncSHA256,
					FileSHA256:    uploaded.FileSHA256,
					FileLength:    proto.Uint64(uint64(len(fileData))),
				}}
			}
			mediaType = "VideoMessage"
		case "ptv":
			if isNewsletter {
				media = &waE2E.Message{PtvMessage: &waE2E.VideoMessage{
					URL:        &uploaded.URL,
					DirectPath: &uploaded.DirectPath,
					Mimetype:   proto.String(mimeType),
					FileSHA256: uploaded.FileSHA256,
					FileLength: &uploaded.FileLength,
				}}
			} else {
				media = &waE2E.Message{PtvMessage: &waE2E.VideoMessage{
					URL:           proto.String(uploaded.URL),
					DirectPath:    proto.String(uploaded.DirectPath),
					MediaKey:      uploaded.MediaKey,
					Mimetype:      proto.String(mimeType),
					FileEncSHA256: uploaded.FileEncSHA256,
					FileSHA256:    uploaded.FileSHA256,
					FileLength:    proto.Uint64(uint64(len(fileData))),
				}}
			}
			mediaType = "PtvMessage"
		case "audio":
			if isNewsletter {
				media = &waE2E.Message{AudioMessage: &waE2E.AudioMessage{
					URL:              &uploaded.URL,
					PTT:              proto.Bool(true),
					DirectPath:       &uploaded.DirectPath,
					Mimetype:         proto.String(mimeType),
					FileSHA256:       uploaded.FileSHA256,
					FileLength:       &uploaded.FileLength,
					StreamingSidecar: []byte(*proto.String("QpmXDsU7YLagdg==")),
					Waveform:         []byte(*proto.String("OjAnExISDgsKCAkJBwgkHAQEBBEFAwMNAxAcKCgkFzM0QUE4Jh4eKAoKChcLCwkeFgkJCQo3JiQmIiIRPz8/Ow==")),
					Seconds:          proto.Uint32(uint32(duration)),
				}}
			} else {
				media = &waE2E.Message{AudioMessage: &waE2E.AudioMessage{
					URL:              proto.String(uploaded.URL),
					PTT:              proto.Bool(true),
					DirectPath:       proto.String(uploaded.DirectPath),
					MediaKey:         uploaded.MediaKey,
					Mimetype:         proto.String(mimeType),
					FileEncSHA256:    uploaded.FileEncSHA256,
					FileSHA256:       uploaded.FileSHA256,
					FileLength:       proto.Uint64(uploaded.FileLength),
					StreamingSidecar: []byte(*proto.String("QpmXDsU7YLagdg==")),
					Waveform:         []byte(*proto.String("OjAnExISDgsKCAkJBwgkHAQEBBEFAwMNAxAcKCgkFzM0QUE4Jh4eKAoKChcLCwkeFgkJCQo3JiQmIiIRPz8/Ow==")),
					Seconds:          proto.Uint32(uint32(duration)),
				}}
			}
			mediaType = "AudioMessage"
		case "document":
			// For PDF documents, rasterize page 1 into a JPEG preview thumbnail.
			// A missing pdftoppm or a failure yields nil and the document is
			// sent without a preview instead of failing the request.
			var jpegThumb []byte
			if mimeType == "application/pdf" {
				jpegThumb = makePDFThumbnail(fileData, 200)
			}
			if isNewsletter {
				media = &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{
					URL:           &uploaded.URL,
					FileName:      &data.Filename,
					Caption:       proto.String(data.Caption),
					DirectPath:    &uploaded.DirectPath,
					Mimetype:      proto.String(mimeType),
					FileSHA256:    uploaded.FileSHA256,
					FileLength:    &uploaded.FileLength,
					JPEGThumbnail: jpegThumb,
				}}
			} else {
				media = &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{
					URL:           proto.String(uploaded.URL),
					FileName:      &data.Filename,
					Caption:       proto.String(data.Caption),
					DirectPath:    proto.String(uploaded.DirectPath),
					MediaKey:      uploaded.MediaKey,
					Mimetype:      proto.String(mimeType),
					FileEncSHA256: uploaded.FileEncSHA256,
					FileSHA256:    uploaded.FileSHA256,
					FileLength:    proto.Uint64(uint64(len(fileData))),
					JPEGThumbnail: jpegThumb,
				}}
			}

			if media.GetDocumentMessage().GetCaption() != "" {
				media.DocumentWithCaptionMessage = &waE2E.FutureProofMessage{
					Message: &waE2E.Message{
						DocumentMessage: media.DocumentMessage,
					},
				}
				media.DocumentMessage = nil
			}

			mediaType = "DocumentMessage"
		default:
			return nil, errors.New("invalid media type")
		}

		messageStart := time.Now()
		message, err := s.SendMessage(instance, media, mediaType, &SendDataStruct{
			Id:              data.Id,
			Number:          data.Number,
			Quoted:          data.Quoted,
			Delay:           data.Delay,
			MentionAll:      data.MentionAll,
			MentionedJID:    data.MentionedJID,
			FormatJid:       data.FormatJid,
			MediaHandle:     uploaded.Handle,
			ForwardingScore: data.ForwardingScore,
		})

		if err != nil {
			// Check if it's a client disconnection error
			if strings.Contains(err.Error(), "client disconnected") || strings.Contains(err.Error(), "no active session") {
				s.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] SendMediaUrl failed due to disconnection on attempt %d/%d: %v", instance.Id, attempt, maxRetries, err)
				if attempt < maxRetries {
					waitTime := time.Duration(attempt) * time.Second
					s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Waiting %v before retry", instance.Id, waitTime)
					time.Sleep(waitTime)
					continue
				}
			}
			return nil, err
		}

		s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Mensagem enviada em %v", instance.Id, time.Since(messageStart))

		totalTime := time.Since(startTime)
		s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] SendMediaUrl successful on attempt %d, processo completo em %v", instance.Id, attempt, totalTime)

		return message, nil
	}

	return nil, fmt.Errorf("failed to send media url after %d attempts", maxRetries)
}

func (s *sendService) SendPoll(data *PollStruct, instance *instance_model.Instance) (*MessageSendStruct, error) {
	return s.sendPollWithRetry(data, instance, 3)
}

func (s *sendService) sendPollWithRetry(data *PollStruct, instance *instance_model.Instance, maxRetries int) (*MessageSendStruct, error) {
	for attempt := 1; attempt <= maxRetries; attempt++ {
		s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] SendPoll attempt %d/%d", instance.Id, attempt, maxRetries)

		client, err := s.ensureClientConnectedWithRetry(instance.Id, 2)
		if err != nil {
			if attempt == maxRetries {
				return nil, err
			}
			continue
		}

		msg := client.BuildPollCreation(data.Question, data.Options, data.MaxAnswer)

		message, err := s.SendMessage(instance, msg, "PollCreationMessage", &SendDataStruct{
			Id:           data.Id,
			Number:       data.Number,
			Quoted:       data.Quoted,
			Delay:        data.Delay,
			MentionAll:   data.MentionAll,
			MentionedJID: data.MentionedJID,
			FormatJid:    data.FormatJid,
		})

		if err != nil {
			// Check if it's a client disconnection error
			if strings.Contains(err.Error(), "client disconnected") || strings.Contains(err.Error(), "no active session") {
				s.loggerWrapper.GetLogger(instance.Id).LogWarn("[%s] SendPoll failed due to disconnection on attempt %d/%d: %v", instance.Id, attempt, maxRetries, err)
				if attempt < maxRetries {
					waitTime := time.Duration(attempt) * time.Second
					s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Waiting %v before retry", instance.Id, waitTime)
					time.Sleep(waitTime)
					continue
				}
			}
			return nil, err
		}

		s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] SendPoll successful on attempt %d", instance.Id, attempt)
		return message, nil
	}

	return nil, fmt.Errorf("failed to send poll after %d attempts", maxRetries)
}

func convertToWebP(imageData string) ([]byte, error) {
	var img image.Image
	var err error

	resp, err := http.Get(imageData)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch image from URL: %v", err)
	}
	defer resp.Body.Close()

	img, _, err = image.Decode(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %v", err)
	}

	var webpBuffer bytes.Buffer
	err = webp.Encode(&webpBuffer, img, &webp.Options{Lossless: false, Quality: 80})
	if err != nil {
		return nil, fmt.Errorf("failed to encode image to WebP: %v", err)
	}

	return webpBuffer.Bytes(), nil
}

// convertBytesToWebP re-encodes a STATIC image (jpeg/png/...) already in memory into WebP. Never
// call with data that is already WebP — the decoder fails on animated WebP ("webpDecodeRGBA: failed")
// and would flatten a static WebP to a single frame anyway.
func convertBytesToWebP(data []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %v", err)
	}
	var webpBuffer bytes.Buffer
	if err := webp.Encode(&webpBuffer, img, &webp.Options{Lossless: false, Quality: 80}); err != nil {
		return nil, fmt.Errorf("failed to encode image to WebP: %v", err)
	}
	return webpBuffer.Bytes(), nil
}

// isWebP checks the RIFF/WEBP magic bytes.
func isWebP(data []byte) bool {
	return len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP"
}

// isAnimatedWebP: an animated WebP carries a VP8X chunk (right after the 12-byte RIFF header) with
// the Animation flag (bit 0x02) set; the ANIM chunk scan is a belt-and-suspenders fallback.
func isAnimatedWebP(data []byte) bool {
	if !isWebP(data) {
		return false
	}
	if len(data) >= 21 && string(data[12:16]) == "VP8X" && data[20]&0x02 != 0 {
		return true
	}
	return bytes.Contains(data, []byte("ANIM"))
}

func (s *sendService) SendSticker(data *StickerStruct, instance *instance_model.Instance) (*MessageSendStruct, error) {
	client, err := s.ensureClientConnected(instance.Id)
	if err != nil {
		return nil, err
	}

	if !strings.HasPrefix(data.Sticker, "http") {
		return nil, fmt.Errorf("invalid sticker URL")
	}

	resp, err := http.Get(data.Sticker)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch sticker from URL: %v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read sticker data: %v", err)
	}

	// Stickers on WhatsApp are already WebP: upload UNTOUCHED (re-encoding both fails on animated
	// WebP and would flatten it to one frame). Only convert when the source is some other format.
	var filedata []byte
	isAnimated := false
	if isWebP(raw) {
		filedata = raw
		isAnimated = isAnimatedWebP(raw)
	} else {
		filedata, err = convertBytesToWebP(raw)
		if err != nil {
			return nil, fmt.Errorf("failed to convert image to WebP: %v", err)
		}
	}

	uploaded, err := client.Upload(context.Background(), filedata, whatsmeow.MediaImage)
	if err != nil {
		return nil, fmt.Errorf("failed to upload sticker: %v", err)
	}

	stickerMsg := &waE2E.StickerMessage{
		URL:           proto.String(uploaded.URL),
		DirectPath:    proto.String(uploaded.DirectPath),
		MediaKey:      uploaded.MediaKey,
		Mimetype:      proto.String("image/webp"), // output is always webp now
		FileEncSHA256: uploaded.FileEncSHA256,
		FileSHA256:    uploaded.FileSHA256,
		FileLength:    proto.Uint64(uint64(len(filedata))),
	}
	if isAnimated {
		stickerMsg.IsAnimated = proto.Bool(true)
	}
	msg := &waE2E.Message{StickerMessage: stickerMsg}

	message, err := s.SendMessage(instance, msg, "StickerMessage", &SendDataStruct{
		Id:           data.Id,
		Number:       data.Number,
		Quoted:       data.Quoted,
		Delay:        data.Delay,
		MentionAll:   data.MentionAll,
		MentionedJID: data.MentionedJID,
		FormatJid:    data.FormatJid,
	})
	if err != nil {
		return nil, err
	}

	return message, nil
}

func (s *sendService) SendLocation(data *LocationStruct, instance *instance_model.Instance) (*MessageSendStruct, error) {
	_, err := s.ensureClientConnected(instance.Id)
	if err != nil {
		return nil, err
	}

	msg := &waE2E.Message{LocationMessage: &waE2E.LocationMessage{
		DegreesLatitude:  &data.Latitude,
		DegreesLongitude: &data.Longitude,
		Name:             &data.Name,
		Address:          &data.Address,
	}}

	message, err := s.SendMessage(instance, msg, "LocationMessage", &SendDataStruct{
		Id:           data.Id,
		Number:       data.Number,
		Quoted:       data.Quoted,
		Delay:        data.Delay,
		MentionAll:   data.MentionAll,
		MentionedJID: data.MentionedJID,
		FormatJid:    data.FormatJid,
	})
	if err != nil {
		return nil, err
	}

	return message, nil
}

func (s *sendService) SendContact(data *ContactStruct, instance *instance_model.Instance) (*MessageSendStruct, error) {
	_, err := s.ensureClientConnected(instance.Id)
	if err != nil {
		return nil, err
	}

	VCstring := utils.GenerateVC(utils.VCardStruct{
		FullName:     data.Vcard.FullName,
		Phone:        data.Vcard.Phone,
		Organization: data.Vcard.Organization,
	})

	fmt.Println(VCstring)

	msg := &waE2E.Message{ContactMessage: &waE2E.ContactMessage{
		DisplayName: &data.Vcard.FullName,
		Vcard:       &VCstring,
	}}

	messaged, err := s.SendMessage(instance, msg, "ContactMessage", &SendDataStruct{
		Id:           data.Id,
		Number:       data.Number,
		Quoted:       data.Quoted,
		Delay:        data.Delay,
		MentionAll:   data.MentionAll,
		MentionedJID: data.MentionedJID,
		FormatJid:    data.FormatJid,
	})
	if err != nil {
		return nil, err
	}

	return messaged, nil
}

func mapKeyType(keyType string) string {
	switch keyType {
	case "phone":
		return "PHONE"
	case "email":
		return "EMAIL"
	case "cpf":
		return "CPF"
	case "cnpj":
		return "CNPJ"
	case "random":
		return "EVP"
	default:
		return keyType
	}
}

func (s *sendService) SendButton(data *ButtonStruct, instance *instance_model.Instance) (*MessageSendStruct, error) {
	client, err := s.ensureClientConnected(instance.Id)
	if err != nil {
		return nil, err
	}

	hasReply := false
	hasPix := false
	hasOtherTypes := false
	replyCount := 0

	for _, v := range data.Buttons {
		switch v.Type {
		case "reply":
			hasReply = true
			replyCount++
		case "pix":
			hasPix = true
		default:
			hasOtherTypes = true
		}
	}

	if hasReply {
		if replyCount > 3 {
			return nil, errors.New("máximo de 3 botões do tipo 'reply' permitidos")
		}
		if hasOtherTypes {
			return nil, errors.New("botões do tipo 'reply' não podem ser misturados com outros tipos")
		}
	}

	if hasPix {
		if len(data.Buttons) > 1 {
			return nil, errors.New("botão do tipo 'pix' não pode ser combinado com outros botões")
		}
	}

	buttons := []*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{}

	for _, v := range data.Buttons {
		var paramsJSON *string
		var name *string

		switch v.Type {
		case "reply":
			name = proto.String("quick_reply")
			jsonBytes, _ := json.Marshal(map[string]string{"display_text": v.DisplayText, "id": v.Id})
			paramsJSON = proto.String(string(jsonBytes))
		case "copy":
			name = proto.String("cta_copy")
			copyCode := v.CopyCode
			if copyCode == "" {
				copyCode = v.Id
			}
			copyId := v.Id
			if copyId == "" {
				copyId = "copy_" + strconv.FormatInt(time.Now().UnixNano(), 10)
			}
			jsonBytes, _ := json.Marshal(map[string]string{"display_text": v.DisplayText, "id": copyId, "copy_code": copyCode})
			paramsJSON = proto.String(string(jsonBytes))
		case "url":
			name = proto.String("cta_url")
			jsonBytes, _ := json.Marshal(map[string]string{"display_text": v.DisplayText, "url": v.URL, "merchant_url": v.URL})
			paramsJSON = proto.String(string(jsonBytes))
		case "call":
			name = proto.String("cta_call")
			jsonBytes, _ := json.Marshal(map[string]string{"display_text": v.DisplayText, "phone_number": v.PhoneNumber})
			paramsJSON = proto.String(string(jsonBytes))
		case "pix":
			randomId := utils.GenerateRandomString(11)
			name = proto.String("payment_info")
			paymentPayload := map[string]interface{}{
				"currency":     v.Currency,
				"total_amount": map[string]interface{}{"value": 0, "offset": 100},
				"reference_id": randomId,
				"type":         "physical-goods",
				"order": map[string]interface{}{
					"status":     "pending",
					"subtotal":   map[string]interface{}{"value": 0, "offset": 100},
					"order_type": "ORDER",
					"items": []map[string]interface{}{
						{
							"name":        "",
							"amount":      map[string]interface{}{"value": 0, "offset": 100},
							"quantity":    0,
							"sale_amount": map[string]interface{}{"value": 0, "offset": 100},
						},
					},
				},
				"payment_settings": []map[string]interface{}{
					{
						"type": "pix_static_code",
						"pix_static_code": map[string]string{
							"merchant_name": v.Name,
							"key":           v.Key,
							"key_type":      mapKeyType(v.KeyType),
						},
					},
				},
				"share_payment_status": false,
			}
			jsonBytes, _ := json.Marshal(paymentPayload)
			paramsJSON = proto.String(string(jsonBytes))
		}

		buttons = append(buttons, &waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
			Name:             name,
			ButtonParamsJSON: paramsJSON,
		})
	}

	templateId := strconv.FormatInt(time.Now().UnixNano()/1000000, 10)
	messageParamsJSON := `{"from":"api","templateId":` + templateId + `}`

	// MessageSecret (32 random bytes) — required for iOS to render interactive messages.
	btnMsgSecret := make([]byte, 32)
	_, _ = crypto_rand.Read(btnMsgSecret)

	var msg *waE2E.Message
	var msgType string

	if hasReply && !hasOtherTypes && !hasPix {
		// Reply-only: native ButtonsMessage wrapped in DocumentWithCaptionMessage (Baileys PR #36).
		var replyButtons []*waE2E.ButtonsMessage_Button
		for _, v := range data.Buttons {
			replyButtons = append(replyButtons, &waE2E.ButtonsMessage_Button{
				ButtonID: proto.String(v.Id),
				ButtonText: &waE2E.ButtonsMessage_Button_ButtonText{
					DisplayText: proto.String(v.DisplayText),
				},
				Type: waE2E.ButtonsMessage_Button_RESPONSE.Enum(),
			})
		}

		buttonsMsg := &waE2E.ButtonsMessage{
			ContentText: proto.String(data.Description),
			FooterText:  proto.String(data.Footer),
			HeaderType:  waE2E.ButtonsMessage_EMPTY.Enum(),
			Buttons:     replyButtons,
		}

		// Optional media header (image or video URL).
		if data.ImageUrl != "" {
			if resp, err := http.Get(data.ImageUrl); err == nil {
				fileData, readErr := io.ReadAll(resp.Body)
				resp.Body.Close()
				if readErr == nil {
					if uploaded, upErr := client.Upload(context.Background(), fileData, whatsmeow.MediaImage); upErr == nil {
						buttonsMsg.HeaderType = waE2E.ButtonsMessage_IMAGE.Enum()
						buttonsMsg.Header = &waE2E.ButtonsMessage_ImageMessage{
							ImageMessage: &waE2E.ImageMessage{
								URL:           proto.String(uploaded.URL),
								DirectPath:    proto.String(uploaded.DirectPath),
								MediaKey:      uploaded.MediaKey,
								Mimetype:      proto.String("image/jpeg"),
								FileEncSHA256: uploaded.FileEncSHA256,
								FileSHA256:    uploaded.FileSHA256,
								FileLength:    proto.Uint64(uint64(len(fileData))),
							},
						}
					}
				}
			}
		} else if data.VideoUrl != "" {
			if resp, err := http.Get(data.VideoUrl); err == nil {
				fileData, readErr := io.ReadAll(resp.Body)
				resp.Body.Close()
				if readErr == nil {
					if uploaded, upErr := client.Upload(context.Background(), fileData, whatsmeow.MediaVideo); upErr == nil {
						buttonsMsg.HeaderType = waE2E.ButtonsMessage_VIDEO.Enum()
						buttonsMsg.Header = &waE2E.ButtonsMessage_VideoMessage{
							VideoMessage: &waE2E.VideoMessage{
								URL:           proto.String(uploaded.URL),
								DirectPath:    proto.String(uploaded.DirectPath),
								MediaKey:      uploaded.MediaKey,
								Mimetype:      proto.String("video/mp4"),
								FileEncSHA256: uploaded.FileEncSHA256,
								FileSHA256:    uploaded.FileSHA256,
								FileLength:    proto.Uint64(uint64(len(fileData))),
							},
						}
					}
				}
			}
		}

		msg = &waE2E.Message{
			DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
				Message: &waE2E.Message{
					ButtonsMessage: buttonsMsg,
				},
			},
			MessageContextInfo: &waE2E.MessageContextInfo{
				MessageSecret: btnMsgSecret,
			},
		}
		msgType = "ButtonsMessage"
	} else if hasPix {
		// Pix: NativeFlowMessage wrapped in DocumentWithCaptionMessage.
		paymentMsgParams := `{"native_flow_name":"order_details","version":1}`

		var interactiveBody *waE2E.InteractiveMessage_Body
		if data.Title != "" {
			bodyText := data.Title
			interactiveBody = &waE2E.InteractiveMessage_Body{Text: &bodyText}
		}

		msg = &waE2E.Message{
			DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
				Message: &waE2E.Message{
					InteractiveMessage: &waE2E.InteractiveMessage{
						Body: interactiveBody,
						InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{
							NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
								Buttons:           buttons,
								MessageParamsJSON: &paymentMsgParams,
								MessageVersion:    proto.Int32(1),
							},
						},
					},
				},
			},
			MessageContextInfo: &waE2E.MessageContextInfo{
				MessageSecret: btnMsgSecret,
			},
		}
		msgType = "InteractiveMessage"
	} else {
		// Mixed CTA buttons (url/copy/call): NativeFlowMessage wrapped in DocumentWithCaptionMessage.
		body := func() string {
			t := "*" + data.Title + "*"
			if data.Description != "" {
				t += "\n\n" + data.Description + "\n"
			}
			return t
		}()

		msg = &waE2E.Message{
			DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
				Message: &waE2E.Message{
					InteractiveMessage: &waE2E.InteractiveMessage{
						Body: &waE2E.InteractiveMessage_Body{
							Text: &body,
						},
						Footer: &waE2E.InteractiveMessage_Footer{
							Text: &data.Footer,
						},
						InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{
							NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
								Buttons:           buttons,
								MessageParamsJSON: &messageParamsJSON,
								MessageVersion:    proto.Int32(1),
							},
						},
					},
				},
			},
			MessageContextInfo: &waE2E.MessageContextInfo{
				MessageSecret: btnMsgSecret,
			},
		}
		msgType = "InteractiveMessage"
	}

	// Build biz/bot nodes injected directly in the XMPP stanza — required for mobile rendering.
	// Reply-only buttons get <biz><buttons/></biz>; CTA/Pix get <biz><interactive type="native_flow" v="1"><native_flow name="X"/></interactive></biz>.
	// The <bot biz_bot="1"/> node is required for 1:1 chats (skipped on groups).
	var bizInteractiveContent waBinary.Node
	if hasReply && !hasOtherTypes && !hasPix {
		bizInteractiveContent = waBinary.Node{
			Tag: "interactive",
			Attrs: waBinary.Attrs{
				"type": "native_flow",
				"v":    "1",
			},
			Content: []waBinary.Node{{
				Tag: "native_flow",
				Attrs: waBinary.Attrs{
					"name": "quick_reply",
				},
			}},
		}
	} else if hasPix {
		bizInteractiveContent = waBinary.Node{
			Tag: "interactive",
			Attrs: waBinary.Attrs{
				"type": "native_flow",
				"v":    "1",
			},
			Content: []waBinary.Node{{
				Tag: "native_flow",
				Attrs: waBinary.Attrs{
					"name": "payment_info",
				},
			}},
		}
	} else {
		// Mixed CTA buttons (url/copy/call) — name="mixed" is the WhatsApp convention.
		bizInteractiveContent = waBinary.Node{
			Tag: "interactive",
			Attrs: waBinary.Attrs{
				"type": "native_flow",
				"v":    "1",
			},
			Content: []waBinary.Node{{
				Tag: "native_flow",
				Attrs: waBinary.Attrs{
					"name": "mixed",
				},
			}},
		}
	}

	bizNodes := []waBinary.Node{
		{
			Tag:     "biz",
			Content: []waBinary.Node{bizInteractiveContent},
		},
	}
	if !strings.Contains(data.Number, "@g.us") {
		bizNodes = append(bizNodes, waBinary.Node{
			Tag:   "bot",
			Attrs: waBinary.Attrs{"biz_bot": "1"},
		})
	}

	// Route through centralized SendMessage for ContextInfo, webhooks, quotes, mentions.
	message, err := s.SendMessage(instance, msg, msgType, &SendDataStruct{
		Number:          data.Number,
		Delay:           data.Delay,
		MentionAll:      data.MentionAll,
		MentionedJID:    data.MentionedJID,
		FormatJid:       data.FormatJid,
		Quoted:          data.Quoted,
		AdditionalNodes: &bizNodes,
	})
	if err != nil {
		s.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Error sending button message: %v", instance.Id, err)
		return nil, err
	}

	return message, nil
}

func stringPointer(s string) *string {
	return &s
}

// makeJPEGThumbnail decodes raw image bytes and produces a small JPEG
// thumbnail suitable for the JPEGThumbnail field of WhatsApp media messages.
// The thumbnail keeps the original aspect ratio and is capped at maxWidth
// pixels wide. It returns nil if the image cannot be decoded so callers can
// fall back to sending the message without a preview thumbnail.
func makeJPEGThumbnail(fileData []byte, maxWidth int) []byte {
	if maxWidth < 1 {
		maxWidth = 72
	}

	img, _, err := image.Decode(bytes.NewReader(fileData))
	if err != nil {
		return nil
	}

	bounds := img.Bounds()
	srcWidth := bounds.Dx()
	srcHeight := bounds.Dy()
	if srcWidth < 1 || srcHeight < 1 {
		return nil
	}

	thumbWidth := maxWidth
	if srcWidth < thumbWidth {
		thumbWidth = srcWidth
	}
	thumbHeight := int(float64(srcHeight) * float64(thumbWidth) / float64(srcWidth))
	if thumbHeight < 1 {
		thumbHeight = 1
	}

	thumbImg := image.NewRGBA(image.Rect(0, 0, thumbWidth, thumbHeight))
	for y := 0; y < thumbHeight; y++ {
		for x := 0; x < thumbWidth; x++ {
			srcX := x * srcWidth / thumbWidth
			srcY := y * srcHeight / thumbHeight
			thumbImg.Set(x, y, img.At(srcX+bounds.Min.X, srcY+bounds.Min.Y))
		}
	}

	var thumbBuf bytes.Buffer
	if err := jpeg.Encode(&thumbBuf, thumbImg, &jpeg.Options{Quality: 50}); err != nil {
		return nil
	}
	return thumbBuf.Bytes()
}

// makePDFThumbnail rasterizes the first page of a PDF into a JPEG thumbnail
// using the external "pdftoppm" tool (poppler-utils). It returns nil when
// pdftoppm is not installed or rasterization fails, so callers can gracefully
// send the document without a preview instead of failing the request.
func makePDFThumbnail(fileData []byte, maxWidth int) []byte {
	if _, err := exec.LookPath("pdftoppm"); err != nil {
		return nil
	}

	scaleWidth := maxWidth
	if scaleWidth < 1 {
		scaleWidth = 72
	}

	// Render only the first page to a PNG on stdout, scaled to scaleWidth.
	// "-scale-to-y -1" keeps the original aspect ratio.
	cmd := exec.Command("pdftoppm",
		"-png",
		"-f", "1",
		"-l", "1",
		"-singlefile",
		"-scale-to-x", strconv.Itoa(scaleWidth),
		"-scale-to-y", "-1",
	)
	cmd.Stdin = bytes.NewReader(fileData)

	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil
	}
	if out.Len() == 0 {
		return nil
	}

	// Re-encode the rendered PNG as a JPEG thumbnail for consistency with images.
	return makeJPEGThumbnail(out.Bytes(), maxWidth)
}

func sectionsToString(data *ListStruct) (string, error) {
	type row struct {
		Header      string `json:"header"`
		Title       string `json:"title"`
		Description string `json:"description"`
		ID          string `json:"id"`
	}

	type listSection struct {
		Title          string `json:"title"`
		HighlightLabel string `json:"highlight_label"`
		Rows           []row  `json:"rows"`
	}

	type list struct {
		Title    string        `json:"title"`
		Sections []listSection `json:"sections"`
	}

	sections := []listSection{}

	for _, s := range data.Sections {
		sectionTitle := s.Title
		if sectionTitle == "" {
			sectionTitle = " "
		}
		rows := []row{}

		for _, r := range s.Rows {
			rowTitle := r.Title
			if rowTitle == "" {
				rowTitle = " "
			}
			rowDesc := r.Description
			if rowDesc == "" {
				rowDesc = " "
			}
			rowId := r.RowId
			if rowId == "" {
				rowId = fmt.Sprintf("row_%d", len(rows))
			}
			rows = append(rows, row{
				Header:      rowTitle,
				Title:       rowTitle,
				Description: rowDesc,
				ID:          rowId,
			})
		}

		section := listSection{
			Title:          sectionTitle,
			HighlightLabel: "",
			Rows:           rows,
		}

		sections = append(sections, section)
	}

	buttonText := data.ButtonText
	if buttonText == "" {
		buttonText = "Ver Menu"
	}

	listData := list{
		Title:    buttonText,
		Sections: sections,
	}

	jsonData, err := json.Marshal(listData)
	if err != nil {
		return "", err
	}

	return string(jsonData), nil
}

func (s *sendService) SendList(data *ListStruct, instance *instance_model.Instance) (*MessageSendStruct, error) {
	// Legacy ListMessage format - works on iOS, Android and Web
	// Matching PAPI Node.js default (non-modern) path exactly

	buttonText := data.ButtonText
	if buttonText == "" {
		buttonText = "Ver Menu"
	}

	// Build sections in legacy ListMessage format
	var sections []*waE2E.ListMessage_Section
	for _, sec := range data.Sections {
		sectionTitle := sec.Title
		if sectionTitle == "" {
			sectionTitle = " "
		}
		var rows []*waE2E.ListMessage_Row
		for i, r := range sec.Rows {
			rowTitle := r.Title
			if rowTitle == "" {
				rowTitle = " "
			}
			rowId := r.RowId
			if rowId == "" {
				rowId = fmt.Sprintf("row_%d_%d", i, len(rows))
			}
			rows = append(rows, &waE2E.ListMessage_Row{
				Title:       proto.String(rowTitle),
				Description: proto.String(r.Description),
				RowID:       proto.String(rowId),
			})
		}
		sections = append(sections, &waE2E.ListMessage_Section{
			Title: proto.String(sectionTitle),
			Rows:  rows,
		})
	}

	listType := waE2E.ListMessage_SINGLE_SELECT
	listMessage := &waE2E.ListMessage{
		Title:       proto.String(data.Title),
		Description: proto.String(data.Description),
		ButtonText:  proto.String(buttonText),
		FooterText:  proto.String(data.FooterText),
		ListType:    &listType,
		Sections:    sections,
	}

	// Wrap ListMessage in DocumentWithCaptionMessage (Baileys PR #36) so modern WhatsApp renders it.
	// MessageSecret (32 random bytes) is required for iOS rendering.
	listMsgSecret := make([]byte, 32)
	_, _ = crypto_rand.Read(listMsgSecret)

	msg := &waE2E.Message{
		DocumentWithCaptionMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				ListMessage: listMessage,
			},
		},
		MessageContextInfo: &waE2E.MessageContextInfo{
			MessageSecret: listMsgSecret,
		},
	}

	// Build biz <list> node — required for mobile rendering of modern lists.
	listBizNodes := []waBinary.Node{
		{
			Tag: "biz",
			Content: []waBinary.Node{{
				Tag: "list",
				Attrs: waBinary.Attrs{
					"v":    "2",
					"type": "single_select",
				},
			}},
		},
	}
	if !strings.Contains(data.Number, "@g.us") {
		listBizNodes = append(listBizNodes, waBinary.Node{
			Tag:   "bot",
			Attrs: waBinary.Attrs{"biz_bot": "1"},
		})
	}

	message, err := s.SendMessage(instance, msg, "ListMessage", &SendDataStruct{
		Number:          data.Number,
		Delay:           data.Delay,
		MentionAll:      data.MentionAll,
		MentionedJID:    data.MentionedJID,
		FormatJid:       data.FormatJid,
		Quoted:          data.Quoted,
		AdditionalNodes: &listBizNodes,
	})

	if err != nil {
		s.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Error sending list: %v", instance.Id, err)
		return nil, err
	}

	s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] List sent to %s", instance.Id, data.Number)
	return message, nil
}

func (s *sendService) SendMessage(instance *instance_model.Instance, msg *waE2E.Message, messageType string, data *SendDataStruct) (*MessageSendStruct, error) {
	s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] SendMessage called for number: %s, type: %s", instance.Id, data.Number, messageType)

	recipient, err := s.validateAndCheckUserExists(data.Number, data.FormatJid, &data.Quoted.MessageID, &data.Quoted.MessageID, instance)
	if err != nil {
		s.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Error validating message fields or user check: %v", instance.Id, err)
		return nil, err
	}

	s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Recipient validated: %s (Server: %s)", instance.Id, recipient.String(), recipient.Server)

	var message string
	if data.Id == "" {
		message = s.clientPointer[instance.Id].GenerateMessageID()
	} else {
		message = data.Id
	}

	if data.Delay > 0 {
		media := ""
		if messageType == "AudioMessage" {
			media = "audio"
		}

		err := s.clientPointer[instance.Id].SendChatPresence(context.Background(), recipient, types.ChatPresence("composing"), types.ChatPresenceMedia(media))
		if err != nil {
			return nil, err
		}

		time.Sleep(time.Duration(data.Delay) * time.Millisecond)

		err = s.clientPointer[instance.Id].SendChatPresence(context.Background(), recipient, types.ChatPresence("paused"), types.ChatPresenceMedia(media))
		if err != nil {
			return nil, err
		}
	}

	isMedia := false

	if data.Quoted.MessageID != "" {
		switch messageType {
		case "ExtendedTextMessage":
			msg.ExtendedTextMessage.ContextInfo = &waE2E.ContextInfo{
				StanzaID:      proto.String(data.Quoted.MessageID),
				Participant:   proto.String(data.Quoted.Participant),
				QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
			}
		case "ImageMessage":
			msg.ImageMessage.ContextInfo = &waE2E.ContextInfo{
				StanzaID:      proto.String(data.Quoted.MessageID),
				Participant:   proto.String(data.Quoted.Participant),
				QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
			}
			isMedia = true
		case "VideoMessage":
			msg.VideoMessage.ContextInfo = &waE2E.ContextInfo{
				StanzaID:      proto.String(data.Quoted.MessageID),
				Participant:   proto.String(data.Quoted.Participant),
				QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
			}
			isMedia = true
		case "PtvMessage":
			msg.PtvMessage.ContextInfo = &waE2E.ContextInfo{
				StanzaID:      proto.String(data.Quoted.MessageID),
				Participant:   proto.String(data.Quoted.Participant),
				QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
			}
			isMedia = true
		case "AudioMessage":
			msg.AudioMessage.ContextInfo = &waE2E.ContextInfo{
				StanzaID:      proto.String(data.Quoted.MessageID),
				Participant:   proto.String(data.Quoted.Participant),
				QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
			}
			isMedia = true
		case "DocumentMessage":
			if msg.DocumentMessage != nil {
				msg.DocumentMessage.ContextInfo = &waE2E.ContextInfo{
					StanzaID:      proto.String(data.Quoted.MessageID),
					Participant:   proto.String(data.Quoted.Participant),
					QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
				}
			} else if msg.DocumentWithCaptionMessage != nil {
				msg.DocumentWithCaptionMessage.Message.DocumentMessage.ContextInfo = &waE2E.ContextInfo{
					StanzaID:      proto.String(data.Quoted.MessageID),
					Participant:   proto.String(data.Quoted.Participant),
					QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
				}
			}
			isMedia = true
		case "PollCreationMessage":
			msg.PollCreationMessage.ContextInfo = &waE2E.ContextInfo{
				StanzaID:      proto.String(data.Quoted.MessageID),
				Participant:   proto.String(data.Quoted.Participant),
				QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
			}
		case "StickerMessage":
			msg.StickerMessage.ContextInfo = &waE2E.ContextInfo{
				StanzaID:      proto.String(data.Quoted.MessageID),
				Participant:   proto.String(data.Quoted.Participant),
				QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
			}
			isMedia = true
		case "LocationMessage":
			msg.LocationMessage.ContextInfo = &waE2E.ContextInfo{
				StanzaID:      proto.String(data.Quoted.MessageID),
				Participant:   proto.String(data.Quoted.Participant),
				QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
			}
		case "ContactMessage":
			msg.ContactMessage.ContextInfo = &waE2E.ContextInfo{
				StanzaID:      proto.String(data.Quoted.MessageID),
				Participant:   proto.String(data.Quoted.Participant),
				QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
			}
		case "InteractiveMessage":
			if msg.InteractiveMessage != nil {
				msg.InteractiveMessage.ContextInfo = &waE2E.ContextInfo{
					StanzaID:      proto.String(data.Quoted.MessageID),
					Participant:   proto.String(data.Quoted.Participant),
					QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
				}
			} else if msg.DocumentWithCaptionMessage != nil &&
				msg.DocumentWithCaptionMessage.Message != nil &&
				msg.DocumentWithCaptionMessage.Message.InteractiveMessage != nil {
				msg.DocumentWithCaptionMessage.Message.InteractiveMessage.ContextInfo = &waE2E.ContextInfo{
					StanzaID:      proto.String(data.Quoted.MessageID),
					Participant:   proto.String(data.Quoted.Participant),
					QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
				}
			}
		case "ListMessage":
			if msg.ListMessage != nil {
				msg.ListMessage.ContextInfo = &waE2E.ContextInfo{
					StanzaID:      proto.String(data.Quoted.MessageID),
					Participant:   proto.String(data.Quoted.Participant),
					QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
				}
			} else if msg.DocumentWithCaptionMessage != nil &&
				msg.DocumentWithCaptionMessage.Message != nil &&
				msg.DocumentWithCaptionMessage.Message.ListMessage != nil {
				msg.DocumentWithCaptionMessage.Message.ListMessage.ContextInfo = &waE2E.ContextInfo{
					StanzaID:      proto.String(data.Quoted.MessageID),
					Participant:   proto.String(data.Quoted.Participant),
					QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
				}
			}
		case "ButtonsMessage":
			if msg.ButtonsMessage != nil {
				msg.ButtonsMessage.ContextInfo = &waE2E.ContextInfo{
					StanzaID:      proto.String(data.Quoted.MessageID),
					Participant:   proto.String(data.Quoted.Participant),
					QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
				}
			} else if msg.DocumentWithCaptionMessage != nil &&
				msg.DocumentWithCaptionMessage.Message != nil &&
				msg.DocumentWithCaptionMessage.Message.ButtonsMessage != nil {
				msg.DocumentWithCaptionMessage.Message.ButtonsMessage.ContextInfo = &waE2E.ContextInfo{
					StanzaID:      proto.String(data.Quoted.MessageID),
					Participant:   proto.String(data.Quoted.Participant),
					QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
				}
			}
		default:
			return nil, fmt.Errorf("invalid messageType: %s", messageType)
		}
	} else {
		switch messageType {
		case "ExtendedTextMessage":
			msg.ExtendedTextMessage.ContextInfo = &waE2E.ContextInfo{}
		case "ImageMessage":
			msg.ImageMessage.ContextInfo = &waE2E.ContextInfo{}
			isMedia = true
		case "VideoMessage":
			msg.VideoMessage.ContextInfo = &waE2E.ContextInfo{}
			isMedia = true
		case "PtvMessage":
			msg.PtvMessage.ContextInfo = &waE2E.ContextInfo{}
			isMedia = true
		case "AudioMessage":
			msg.AudioMessage.ContextInfo = &waE2E.ContextInfo{}
			isMedia = true
		case "DocumentMessage":
			if msg.DocumentMessage != nil {
				msg.DocumentMessage.ContextInfo = &waE2E.ContextInfo{}
			} else if msg.DocumentWithCaptionMessage != nil {
				msg.DocumentWithCaptionMessage.Message.DocumentMessage.ContextInfo = &waE2E.ContextInfo{}
			}
			isMedia = true
		case "PollCreationMessage":
			msg.PollCreationMessage.ContextInfo = &waE2E.ContextInfo{}
		case "StickerMessage":
			msg.StickerMessage.ContextInfo = &waE2E.ContextInfo{}
		case "LocationMessage":
			msg.LocationMessage.ContextInfo = &waE2E.ContextInfo{}
		case "ContactMessage":
			msg.ContactMessage.ContextInfo = &waE2E.ContextInfo{}
		case "InteractiveMessage":
			// ContextInfo already set in SendCarousel/SendButton/SendList
		case "ListMessage":
			// ContextInfo already set in SendList
		case "ButtonsMessage":
			// Reply-only buttons: ContextInfo already set in SendButton
		default:
			return nil, fmt.Errorf("invalid messageType: %s", messageType)
		}
	}

	// Apply ForwardingScore to whichever ContextInfo was set above.
	// WhatsApp renders "Encaminhada" when ContextInfo.ForwardingScore > 0.
	if data.ForwardingScore != nil && *data.ForwardingScore > 0 {
		switch messageType {
		case "ExtendedTextMessage":
			if msg.ExtendedTextMessage != nil && msg.ExtendedTextMessage.ContextInfo != nil {
				msg.ExtendedTextMessage.ContextInfo.ForwardingScore = data.ForwardingScore
				msg.ExtendedTextMessage.ContextInfo.IsForwarded = proto.Bool(true)
			}
		case "ImageMessage":
			if msg.ImageMessage != nil && msg.ImageMessage.ContextInfo != nil {
				msg.ImageMessage.ContextInfo.ForwardingScore = data.ForwardingScore
				msg.ImageMessage.ContextInfo.IsForwarded = proto.Bool(true)
			}
		case "VideoMessage":
			if msg.VideoMessage != nil && msg.VideoMessage.ContextInfo != nil {
				msg.VideoMessage.ContextInfo.ForwardingScore = data.ForwardingScore
				msg.VideoMessage.ContextInfo.IsForwarded = proto.Bool(true)
			}
		case "PtvMessage":
			if msg.PtvMessage != nil && msg.PtvMessage.ContextInfo != nil {
				msg.PtvMessage.ContextInfo.ForwardingScore = data.ForwardingScore
				msg.PtvMessage.ContextInfo.IsForwarded = proto.Bool(true)
			}
		case "AudioMessage":
			if msg.AudioMessage != nil && msg.AudioMessage.ContextInfo != nil {
				msg.AudioMessage.ContextInfo.ForwardingScore = data.ForwardingScore
				msg.AudioMessage.ContextInfo.IsForwarded = proto.Bool(true)
			}
		case "DocumentMessage":
			if msg.DocumentMessage != nil && msg.DocumentMessage.ContextInfo != nil {
				msg.DocumentMessage.ContextInfo.ForwardingScore = data.ForwardingScore
				msg.DocumentMessage.ContextInfo.IsForwarded = proto.Bool(true)
			} else if msg.DocumentWithCaptionMessage != nil && msg.DocumentWithCaptionMessage.Message != nil && msg.DocumentWithCaptionMessage.Message.DocumentMessage != nil && msg.DocumentWithCaptionMessage.Message.DocumentMessage.ContextInfo != nil {
				msg.DocumentWithCaptionMessage.Message.DocumentMessage.ContextInfo.ForwardingScore = data.ForwardingScore
				msg.DocumentWithCaptionMessage.Message.DocumentMessage.ContextInfo.IsForwarded = proto.Bool(true)
			}
		case "PollCreationMessage":
			if msg.PollCreationMessage != nil && msg.PollCreationMessage.ContextInfo != nil {
				msg.PollCreationMessage.ContextInfo.ForwardingScore = data.ForwardingScore
				msg.PollCreationMessage.ContextInfo.IsForwarded = proto.Bool(true)
			}
		case "StickerMessage":
			if msg.StickerMessage != nil && msg.StickerMessage.ContextInfo != nil {
				msg.StickerMessage.ContextInfo.ForwardingScore = data.ForwardingScore
				msg.StickerMessage.ContextInfo.IsForwarded = proto.Bool(true)
			}
		case "LocationMessage":
			if msg.LocationMessage != nil && msg.LocationMessage.ContextInfo != nil {
				msg.LocationMessage.ContextInfo.ForwardingScore = data.ForwardingScore
				msg.LocationMessage.ContextInfo.IsForwarded = proto.Bool(true)
			}
		case "ContactMessage":
			if msg.ContactMessage != nil && msg.ContactMessage.ContextInfo != nil {
				msg.ContactMessage.ContextInfo.ForwardingScore = data.ForwardingScore
				msg.ContactMessage.ContextInfo.IsForwarded = proto.Bool(true)
			}
		case "InteractiveMessage":
			if msg.InteractiveMessage != nil && msg.InteractiveMessage.ContextInfo != nil {
				msg.InteractiveMessage.ContextInfo.ForwardingScore = data.ForwardingScore
				msg.InteractiveMessage.ContextInfo.IsForwarded = proto.Bool(true)
			}
		case "ListMessage":
			if msg.ListMessage != nil && msg.ListMessage.ContextInfo != nil {
				msg.ListMessage.ContextInfo.ForwardingScore = data.ForwardingScore
				msg.ListMessage.ContextInfo.IsForwarded = proto.Bool(true)
			}
		}
	}

	isGroup := strings.Contains(data.Number, "@g.us")
	isNewsletter := strings.Contains(data.Number, "@newsletter")

	// Only try to get participants for actual groups, not newsletters
	if isGroup && !isNewsletter {
		if data.MentionAll {
			groupInfo, err := s.clientPointer[instance.Id].GetGroupInfo(context.Background(), recipient)
			if err != nil {
				return nil, err
			}

			var mentionedJIDs []string
			for _, participant := range groupInfo.Participants {
				mentionedJIDs = append(mentionedJIDs, participant.JID.String())
			}

			switch messageType {
			case "ExtendedTextMessage":
				if msg.ExtendedTextMessage.ContextInfo == nil {
					msg.ExtendedTextMessage.ContextInfo = &waE2E.ContextInfo{}
				}
				msg.ExtendedTextMessage.ContextInfo.MentionedJID = mentionedJIDs
			case "ImageMessage":
				if msg.ImageMessage.ContextInfo == nil {
					msg.ImageMessage.ContextInfo = &waE2E.ContextInfo{}
				}
				msg.ImageMessage.ContextInfo.MentionedJID = mentionedJIDs
			case "VideoMessage":
				if msg.VideoMessage.ContextInfo == nil {
					msg.VideoMessage.ContextInfo = &waE2E.ContextInfo{}
				}
				msg.VideoMessage.ContextInfo.MentionedJID = mentionedJIDs
			case "PtvMessage":
				if msg.PtvMessage.ContextInfo == nil {
					msg.PtvMessage.ContextInfo = &waE2E.ContextInfo{}
				}
				msg.PtvMessage.ContextInfo.MentionedJID = mentionedJIDs
			case "AudioMessage":
				if msg.AudioMessage.ContextInfo == nil {
					msg.AudioMessage.ContextInfo = &waE2E.ContextInfo{}
				}
				msg.AudioMessage.ContextInfo.MentionedJID = mentionedJIDs
			case "DocumentMessage":
				if msg.DocumentMessage.ContextInfo == nil {
					msg.DocumentMessage.ContextInfo = &waE2E.ContextInfo{}
				}
				msg.DocumentMessage.ContextInfo.MentionedJID = mentionedJIDs
			case "PollCreationMessage":
				if msg.PollCreationMessage.ContextInfo == nil {
					msg.PollCreationMessage.ContextInfo = &waE2E.ContextInfo{}
				}
				msg.PollCreationMessage.ContextInfo.MentionedJID = mentionedJIDs
			case "StickerMessage":
				if msg.StickerMessage.ContextInfo == nil {
					msg.StickerMessage.ContextInfo = &waE2E.ContextInfo{}
				}
				msg.StickerMessage.ContextInfo.MentionedJID = mentionedJIDs
			case "LocationMessage":
				if msg.LocationMessage.ContextInfo == nil {
					msg.LocationMessage.ContextInfo = &waE2E.ContextInfo{}
				}
				msg.LocationMessage.ContextInfo.MentionedJID = mentionedJIDs
			case "ContactMessage":
				if msg.ContactMessage.ContextInfo == nil {
					msg.ContactMessage.ContextInfo = &waE2E.ContextInfo{}
				}
				msg.ContactMessage.ContextInfo.MentionedJID = mentionedJIDs
			}

		}

		if len(data.MentionedJID) > 0 {
			switch messageType {
			case "ExtendedTextMessage":
				if msg.ExtendedTextMessage.ContextInfo == nil {
					msg.ExtendedTextMessage.ContextInfo = &waE2E.ContextInfo{}
				}
				msg.ExtendedTextMessage.ContextInfo.MentionedJID = data.MentionedJID
			case "ImageMessage":
				if msg.ImageMessage.ContextInfo == nil {
					msg.ImageMessage.ContextInfo = &waE2E.ContextInfo{}
				}
				msg.ImageMessage.ContextInfo.MentionedJID = data.MentionedJID
			case "VideoMessage":
				if msg.VideoMessage.ContextInfo == nil {
					msg.VideoMessage.ContextInfo = &waE2E.ContextInfo{}
				}
				msg.VideoMessage.ContextInfo.MentionedJID = data.MentionedJID
			case "PtvMessage":
				if msg.PtvMessage.ContextInfo == nil {
					msg.PtvMessage.ContextInfo = &waE2E.ContextInfo{}
				}
				msg.PtvMessage.ContextInfo.MentionedJID = data.MentionedJID
			case "AudioMessage":
				if msg.AudioMessage.ContextInfo == nil {
					msg.AudioMessage.ContextInfo = &waE2E.ContextInfo{}
				}
				msg.AudioMessage.ContextInfo.MentionedJID = data.MentionedJID
			case "DocumentMessage":
				if msg.DocumentMessage.ContextInfo == nil {
					msg.DocumentMessage.ContextInfo = &waE2E.ContextInfo{}
				}
				msg.DocumentMessage.ContextInfo.MentionedJID = data.MentionedJID
			case "PollCreationMessage":
				if msg.PollCreationMessage.ContextInfo == nil {
					msg.PollCreationMessage.ContextInfo = &waE2E.ContextInfo{}
				}
				msg.PollCreationMessage.ContextInfo.MentionedJID = data.MentionedJID
			case "StickerMessage":
				if msg.StickerMessage.ContextInfo == nil {
					msg.StickerMessage.ContextInfo = &waE2E.ContextInfo{}
				}
				msg.StickerMessage.ContextInfo.MentionedJID = data.MentionedJID
			case "LocationMessage":
				if msg.LocationMessage.ContextInfo == nil {
					msg.LocationMessage.ContextInfo = &waE2E.ContextInfo{}
				}
				msg.LocationMessage.ContextInfo.MentionedJID = data.MentionedJID
			case "ContactMessage":
				if msg.ContactMessage.ContextInfo == nil {
					msg.ContactMessage.ContextInfo = &waE2E.ContextInfo{}
				}
				msg.ContactMessage.ContextInfo.MentionedJID = data.MentionedJID
			}
		}
	}

	recipient.User = strings.ReplaceAll(recipient.User, "+", "")

	s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Sending message to %s with ID %s", instance.Id, recipient.String(), message)

	// Preparar extra parameters para o envio
	sendExtra := whatsmeow.SendRequestExtra{ID: message}

	// Para newsletters/canais, adicionar o MediaHandle se houver mídia
	if recipient.Server == "newsletter" && data.MediaHandle != "" {
		sendExtra.MediaHandle = data.MediaHandle
		s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Newsletter detected, using MediaHandle: %s", instance.Id, data.MediaHandle)
	}

	// Injetar nodes biz/bot customizados (PIX, botões interativos, etc.) no stanza XMPP.
	if data.AdditionalNodes != nil {
		sendExtra.AdditionalNodes = data.AdditionalNodes
	}

	response, err := s.clientPointer[instance.Id].SendMessage(context.Background(), recipient, msg, sendExtra)
	if err != nil {
		s.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Error sending message: %v", instance.Id, err)
		return nil, err
	}

	s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Message sent successfully! ServerID: %d", instance.Id, response.ServerID)

	messageInfo := types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat:     recipient,
			Sender:   *s.clientPointer[instance.Id].Store.ID,
			IsFromMe: true,
			IsGroup:  isGroup,
		},
		ID:        message,
		Timestamp: time.Now(),
		ServerID:  response.ServerID,
		Type:      messageType,
	}

	messageSent := &MessageSendStruct{
		Info:    messageInfo,
		Message: msg,
		MessageContextInfo: &waE2E.ContextInfo{
			StanzaID:      proto.String(data.Quoted.MessageID),
			Participant:   proto.String(data.Quoted.Participant),
			QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
		},
	}

	postMap := make(map[string]interface{})
	postMap["event"] = "SendMessage"

	// Convertendo o MessageSendStruct para map antes de atribuir
	messageData := make(map[string]interface{})
	messageData["Info"] = messageSent.Info

	// Convertendo a mensagem para map usando json marshal/unmarshal
	msgBytes, err := json.Marshal(messageSent.Message)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message: %v", err)
	}

	var msgMap map[string]interface{}
	if err := json.Unmarshal(msgBytes, &msgMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %v", err)
	}

	messageData["Message"] = msgMap
	messageData["MessageContextInfo"] = messageSent.MessageContextInfo

	postMap["data"] = messageData

	if isMedia && s.config.WebhookFiles {
		var data []byte
		var err error

		img := msg.GetImageMessage()
		audio := msg.GetAudioMessage()
		document := msg.GetDocumentMessage()
		video := msg.GetVideoMessage()
		sticker := msg.GetStickerMessage()

		if img != nil {
			data, err = s.clientPointer[instance.Id].Download(context.Background(), img)
		} else if audio != nil {
			data, err = s.clientPointer[instance.Id].Download(context.Background(), audio)
		} else if document != nil {
			data, err = s.clientPointer[instance.Id].Download(context.Background(), document)
		} else if video != nil {
			data, err = s.clientPointer[instance.Id].Download(context.Background(), video)
		} else if sticker != nil {
			data, err = s.clientPointer[instance.Id].Download(context.Background(), sticker)

			webpReader := bytes.NewReader(data)
			img, err := webp.Decode(webpReader)
			if err == nil {
				var pngBuffer bytes.Buffer
				err = png.Encode(&pngBuffer, img)
				if err == nil {
					data = pngBuffer.Bytes()
				}
			}
		}

		if err == nil {
			// Acessando o Message do map já convertido
			messageMap := msgMap
			if messageMap == nil {
				messageMap = make(map[string]interface{})
			}

			encodeData := base64.StdEncoding.EncodeToString(data)
			messageMap["base64"] = encodeData

			messageData["Message"] = messageMap
		}
	}

	postMap["instanceToken"] = instance.Token
	postMap["instanceId"] = instance.Id
	postMap["instanceName"] = instance.Name

	var queueName string

	if _, ok := postMap["event"]; ok {
		queueName = strings.ToLower(fmt.Sprintf("%s.%s", instance.Id, postMap["event"]))
	}

	values, err := json.Marshal(postMap)
	if err != nil {
		s.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Failed to marshal JSON for queue", instance.Id)
		return nil, err
	}

	go s.whatsmeowService.CallWebhook(instance, queueName, values)

	if s.config.AmqpGlobalEnabled || s.config.NatsGlobalEnabled {
		go s.whatsmeowService.SendToGlobalQueues(postMap["event"].(string), values, instance.Id)
	}

	s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Message sent to %s", instance.Id, data.Number)
	return messageSent, nil
}

func (s *sendService) SendCarousel(data *CarouselStruct, instance *instance_model.Instance) (*MessageSendStruct, error) {
	client, err := s.ensureClientConnected(instance.Id)
	if err != nil {
		return nil, err
	}

	formatJid := true
	if data.FormatJid != nil {
		formatJid = *data.FormatJid
	}

	var recipient types.JID
	var ok bool
	recipient, ok = utils.ParseJID(data.Number)
	if !ok && formatJid {
		s.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Error validating message fields", instance.Id)
		return nil, errors.New("invalid phone number")
	} else if !ok && !formatJid {
		recipient = types.JID{
			User:   data.Number,
			Server: types.DefaultUserServer,
		}
	}

	// Build carousel cards
	cards := make([]*waE2E.InteractiveMessage, len(data.Cards))
	messageVersion := int32(1)

	s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Building carousel for %s with %d cards", instance.Id, recipient.String(), len(data.Cards))

	for i, card := range data.Cards {
		// Each card MUST have both header and body for carousel to work
		interactiveCard := &waE2E.InteractiveMessage{
			Body: &waE2E.InteractiveMessage_Body{
				Text: proto.String(card.Body.Text),
			},
			Header: &waE2E.InteractiveMessage_Header{
				Title:              proto.String(card.Header.Title),
				Subtitle:           proto.String(card.Header.Subtitle),
				HasMediaAttachment: proto.Bool(false),
			},
		}

		// Add media to header if URL provided
		if card.Header.ImageUrl != "" || card.Header.VideoUrl != "" {
			header := interactiveCard.Header

			if card.Header.ImageUrl != "" {
				// Download image
				resp, err := http.Get(card.Header.ImageUrl)
				if err == nil {
					defer resp.Body.Close()
					fileData, err := io.ReadAll(resp.Body)
					if err == nil {
						uploaded, err := client.Upload(context.Background(), fileData, whatsmeow.MediaImage)
						if err == nil {
							// Generate JPEG thumbnail for iOS compatibility
							jpegThumb := makeJPEGThumbnail(fileData, 72)

							header.HasMediaAttachment = proto.Bool(true)
							header.Media = &waE2E.InteractiveMessage_Header_ImageMessage{
								ImageMessage: &waE2E.ImageMessage{
									URL:           proto.String(uploaded.URL),
									DirectPath:    proto.String(uploaded.DirectPath),
									MediaKey:      uploaded.MediaKey,
									Mimetype:      proto.String("image/jpeg"),
									FileEncSHA256: uploaded.FileEncSHA256,
									FileSHA256:    uploaded.FileSHA256,
									FileLength:    proto.Uint64(uint64(len(fileData))),
									JPEGThumbnail: jpegThumb,
								},
							}
						}
					}
				}
			} else if card.Header.VideoUrl != "" {
				// Download and upload video
				resp, err := http.Get(card.Header.VideoUrl)
				if err == nil {
					defer resp.Body.Close()
					fileData, err := io.ReadAll(resp.Body)
					if err == nil {
						uploaded, err := client.Upload(context.Background(), fileData, whatsmeow.MediaVideo)
						if err == nil {
							header.HasMediaAttachment = proto.Bool(true)
							header.Media = &waE2E.InteractiveMessage_Header_VideoMessage{
								VideoMessage: &waE2E.VideoMessage{
									URL:           proto.String(uploaded.URL),
									DirectPath:    proto.String(uploaded.DirectPath),
									MediaKey:      uploaded.MediaKey,
									Mimetype:      proto.String("video/mp4"),
									FileEncSHA256: uploaded.FileEncSHA256,
									FileSHA256:    uploaded.FileSHA256,
									FileLength:    proto.Uint64(uint64(len(fileData))),
								},
							}
						}
					}
				}
			}
		}

		// Add footer if exists
		if card.Footer != "" {
			interactiveCard.Footer = &waE2E.InteractiveMessage_Footer{
				Text: proto.String(card.Footer),
			}
		}

		// Add buttons if exist
		if len(card.Buttons) > 0 {
			buttons := make([]*waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton, len(card.Buttons))
			for j, btn := range card.Buttons {
				buttonType := strings.ToUpper(btn.Type)
				if buttonType == "" {
					buttonType = "REPLY" // Default type
				}

				var buttonName string
				var buttonParams string

				switch buttonType {
				case "URL":
					// URL button - opens a link
					buttonName = "cta_url"
					buttonParams = fmt.Sprintf(`{"display_text":"%s","url":"%s"}`, btn.DisplayText, btn.Id)
				case "CALL":
					// Call button - initiates a phone call
					buttonName = "cta_call"
					buttonParams = fmt.Sprintf(`{"display_text":"%s","phone_number":"%s"}`, btn.DisplayText, btn.Id)
				case "COPY":
					// Copy button - copies text to clipboard
					buttonName = "cta_copy"
					buttonParams = fmt.Sprintf(`{"display_text":"%s","copy_code":"%s"}`, btn.DisplayText, btn.CopyCode)
				case "REPLY":
					fallthrough
				default:
					// Quick reply button (default)
					buttonName = "quick_reply"
					buttonParams = fmt.Sprintf(`{"display_text":"%s","id":"%s"}`, btn.DisplayText, btn.Id)
				}

				buttons[j] = &waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
					Name:             proto.String(buttonName),
					ButtonParamsJSON: proto.String(buttonParams),
				}
			}

			// Cards in carousel: do NOT set MessageParamsJSON or MessageVersion
			// (matching PAPI Node.js behavior for iOS compatibility)
			interactiveCard.InteractiveMessage = &waE2E.InteractiveMessage_NativeFlowMessage_{
				NativeFlowMessage: &waE2E.InteractiveMessage_NativeFlowMessage{
					Buttons: buttons,
				},
			}
		}

		cards[i] = interactiveCard
	}

	// Build carousel message (do NOT set CarouselCardType - matching PAPI Node.js for iOS)
	interactiveMsg := &waE2E.InteractiveMessage{
		InteractiveMessage: &waE2E.InteractiveMessage_CarouselMessage_{
			CarouselMessage: &waE2E.InteractiveMessage_CarouselMessage{
				Cards:          cards,
				MessageVersion: &messageVersion,
			},
		},
	}

	// Add body if provided (main message above carousel)
	if data.Body != "" {
		interactiveMsg.Body = &waE2E.InteractiveMessage_Body{
			Text: proto.String(data.Body),
		}
	}

	// Add footer if provided (text below carousel)
	if data.Footer != "" {
		interactiveMsg.Footer = &waE2E.InteractiveMessage_Footer{
			Text: proto.String(data.Footer),
		}
	}

	// ContextInfo is REQUIRED for iOS compatibility
	// Even if empty, iOS requires this field to display carousel
	contextInfo := &waE2E.ContextInfo{}

	// Add quoted message if exists
	if data.Quoted.MessageID != "" {
		contextInfo.StanzaID = proto.String(data.Quoted.MessageID)
		if data.Quoted.Participant != "" {
			participantJID, ok := utils.ParseJID(data.Quoted.Participant)
			if ok {
				contextInfo.Participant = proto.String(participantJID.String())
			}
		}
	}

	// Always set ContextInfo (required for iOS)
	interactiveMsg.ContextInfo = contextInfo

	// Build final message with MessageContextInfo for proper notification delivery
	msg := &waE2E.Message{
		InteractiveMessage: interactiveMsg,
		MessageContextInfo: &waE2E.MessageContextInfo{
			DeviceListMetadata: &waE2E.DeviceListMetadata{},
		},
	}

	message, err := s.SendMessage(instance, msg, "InteractiveMessage", &SendDataStruct{
		Number: data.Number,
		Delay:  data.Delay,
	})

	if err != nil {
		s.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Error sending carousel: %v", instance.Id, err)
		return nil, err
	}

	s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Carousel sent to %s with %d cards", instance.Id, data.Number, len(data.Cards))
	return message, nil
}

func (s *sendService) SendStatusText(data *StatusTextStruct, instance *instance_model.Instance) (*MessageSendStruct, error) {
	client, err := s.ensureClientConnected(instance.Id)
	if err != nil {
		return nil, err
	}

	if data.Text == "" {
		return nil, errors.New("text is required")
	}

	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: &data.Text,
		},
	}

	messageID := data.Id
	if messageID == "" {
		messageID = client.GenerateMessageID()
	}

	recipient := types.NewJID("status", "broadcast")

	response, err := client.SendMessage(context.Background(), recipient, msg, whatsmeow.SendRequestExtra{ID: messageID})
	if err != nil {
		return nil, err
	}

	messageInfo := types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat:     recipient,
			Sender:   *client.Store.ID,
			IsFromMe: true,
			IsGroup:  false,
		},
		ID:        messageID,
		Timestamp: time.Now(),
		ServerID:  response.ServerID,
		Type:      "StatusTextMessage",
	}

	messageSent := &MessageSendStruct{
		Info:    messageInfo,
		Message: msg,
		MessageContextInfo: &waE2E.ContextInfo{
			StanzaID:      proto.String(""),
			Participant:   proto.String(""),
			QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
		},
	}

	s.sendStatusWebhook(messageSent, instance, "text")
	s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Status text sent successfully", instance.Id)
	return messageSent, nil
}

func (s *sendService) SendStatusMediaUrl(data *StatusMediaStruct, instance *instance_model.Instance) (*MessageSendStruct, error) {
	client, err := s.ensureClientConnected(instance.Id)
	if err != nil {
		return nil, err
	}

	if data.Url == "" {
		return nil, errors.New("url is required")
	}
	if data.Type != "image" && data.Type != "video" {
		return nil, errors.New("type must be 'image' or 'video'")
	}

	req, err := http.NewRequest("GET", data.Url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Evolution-GO/1.0")

	httpClient := &http.Client{}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download file from URL: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("failed to download file: HTTP status %d", resp.StatusCode)
	}

	fileData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return s.sendStatusMedia(client, data, fileData, instance)
}

func (s *sendService) SendStatusMediaFile(data *StatusMediaStruct, fileData []byte, instance *instance_model.Instance) (*MessageSendStruct, error) {
	client, err := s.ensureClientConnected(instance.Id)
	if err != nil {
		return nil, err
	}

	if data.Type != "image" && data.Type != "video" {
		return nil, errors.New("type must be 'image' or 'video'")
	}

	return s.sendStatusMedia(client, data, fileData, instance)
}

func (s *sendService) sendStatusMedia(client *whatsmeow.Client, data *StatusMediaStruct, fileData []byte, instance *instance_model.Instance) (*MessageSendStruct, error) {
	mime, _ := mimetype.DetectReader(bytes.NewReader(fileData))
	mimeType := mime.String()

	var uploadType whatsmeow.MediaType
	switch data.Type {
	case "image":
		if mimeType != "image/jpeg" && mimeType != "image/png" && mimeType != "image/webp" {
			return nil, fmt.Errorf("invalid file format: '%s'. Only 'image/jpeg', 'image/png' and 'image/webp' are accepted", mimeType)
		}
		if mimeType == "image/webp" {
			mimeType = "image/jpeg"
		}
		uploadType = whatsmeow.MediaImage
	case "video":
		if mimeType != "video/mp4" {
			return nil, fmt.Errorf("invalid file format: '%s'. Only 'video/mp4' is accepted", mimeType)
		}
		uploadType = whatsmeow.MediaVideo
	default:
		return nil, errors.New("invalid media type")
	}

	uploaded, err := client.Upload(context.Background(), fileData, uploadType)
	if err != nil {
		return nil, err
	}

	s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Status media uploaded, size: %d", instance.Id, uploaded.FileLength)

	var media *waE2E.Message
	var mediaType string

	switch data.Type {
	case "image":
		// Generate a JPEG preview thumbnail so status/story images render an
		// inline preview instead of the gray camera placeholder. On failure
		// jpegThumb is nil and the status is posted without a preview.
		jpegThumb := makeJPEGThumbnail(fileData, 72)
		media = &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
			Caption:       proto.String(data.Caption),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(mimeType),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(fileData))),
			JPEGThumbnail: jpegThumb,
		}}
		mediaType = "ImageMessage"
	case "video":
		media = &waE2E.Message{VideoMessage: &waE2E.VideoMessage{
			Caption:       proto.String(data.Caption),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			Mimetype:      proto.String(mimeType),
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(fileData))),
		}}
		mediaType = "VideoMessage"
	}

	messageID := data.Id
	if messageID == "" {
		messageID = client.GenerateMessageID()
	}

	recipient := types.NewJID("status", "broadcast")

	response, err := client.SendMessage(context.Background(), recipient, media, whatsmeow.SendRequestExtra{ID: messageID})
	if err != nil {
		return nil, err
	}

	messageInfo := types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat:     recipient,
			Sender:   *client.Store.ID,
			IsFromMe: true,
			IsGroup:  false,
		},
		ID:        messageID,
		Timestamp: time.Now(),
		ServerID:  response.ServerID,
		Type:      mediaType,
	}

	messageSent := &MessageSendStruct{
		Info:    messageInfo,
		Message: media,
		MessageContextInfo: &waE2E.ContextInfo{
			StanzaID:      proto.String(""),
			Participant:   proto.String(""),
			QuotedMessage: &waE2E.Message{Conversation: proto.String("")},
		},
	}

	s.sendStatusWebhook(messageSent, instance, "media")
	return messageSent, nil
}

func (s *sendService) sendStatusWebhook(messageSent *MessageSendStruct, instance *instance_model.Instance, messageType string) {
	postMap := make(map[string]interface{})
	postMap["event"] = "SendStatus"
	messageData := make(map[string]interface{})
	messageData["Info"] = messageSent.Info
	msgBytes, err := json.Marshal(messageSent.Message)
	if err != nil {
		s.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Failed to marshal status message: %v", instance.Id, err)
		return
	}
	var msgMap map[string]interface{}
	if err := json.Unmarshal(msgBytes, &msgMap); err != nil {
		s.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Failed to unmarshal status message: %v", instance.Id, err)
		return
	}
	messageData["Message"] = msgMap
	messageData["MessageContextInfo"] = messageSent.MessageContextInfo
	postMap["data"] = messageData
	postMap["instanceToken"] = instance.Token
	postMap["instanceId"] = instance.Id
	postMap["instanceName"] = instance.Name

	values, err := json.Marshal(postMap)
	if err != nil {
		s.loggerWrapper.GetLogger(instance.Id).LogError("[%s] Failed to marshal webhook payload: %v", instance.Id, err)
		return
	}
	go s.whatsmeowService.CallWebhook(instance, "sendstatus", values)
	if s.config.AmqpGlobalEnabled || s.config.NatsGlobalEnabled {
		go s.whatsmeowService.SendToGlobalQueues("SendStatus", values, instance.Id)
	}
	s.loggerWrapper.GetLogger(instance.Id).LogInfo("[%s] Status %s sent successfully", instance.Id, messageType)
}

func NewSendService(
	clientPointer map[string]*whatsmeow.Client,
	whatsmeowService whatsmeow_service.WhatsmeowService,
	config *config.Config,
	loggerWrapper *logger_wrapper.LoggerManager,
) SendService {
	return &sendService{
		clientPointer:    clientPointer,
		whatsmeowService: whatsmeowService,
		config:           config,
		loggerWrapper:    loggerWrapper,
	}
}
