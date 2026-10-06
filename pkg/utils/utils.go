package utils

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/gomessguii/logger"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/proto/waE2E"
	whatsmeow_types "go.mau.fi/whatsmeow/types"
	"golang.org/x/exp/rand"
	"golang.org/x/net/proxy"
)

type Values struct {
	m map[string]string
}

type VCardStruct struct {
	FullName     string `json:"fullName"`
	Organization string `json:"organization"`
	Phone        string `json:"phone"`
}

func GenerateRandomString(length int) string {
	characters := "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = characters[rand.Intn(len(characters))]
	}
	return string(b)
}

func Find(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

// CreateJID creates a properly formatted WhatsApp JID from a number string
// This function matches the TypeScript createJid functionality with enhanced validation
func CreateJID(number string) (string, error) {
	if number == "" {
		return "", fmt.Errorf("number cannot be empty")
	}

	// Remove timestamp suffix if present
	// number = strings.Split(number, ":")[0]

	// Check if already a valid JID format
	if strings.Contains(number, "@g.us") ||
		strings.Contains(number, "@s.whatsapp.net") ||
		strings.Contains(number, "@lid") ||
		strings.Contains(number, "@broadcast") ||
		strings.Contains(number, "@newsletter") {
		return number, nil
	}

	// Clean the number
	number = strings.ReplaceAll(number, " ", "")
	number = strings.ReplaceAll(number, "+", "")
	number = strings.ReplaceAll(number, "(", "")
	number = strings.ReplaceAll(number, ")", "")
	number = strings.Split(number, ":")[0]

	// Check if it's a group by hyphen and length
	if strings.Contains(number, "-") && len(number) >= 24 {
		// Remove non-digit and non-hyphen characters
		groupID := strings.Map(func(r rune) rune {
			if (r >= '0' && r <= '9') || r == '-' {
				return r
			}
			return -1
		}, number)
		return groupID + "@g.us", nil
	}

	// Check if it's a group by length (18+ digits)
	if len(number) >= 18 {
		// Remove non-digit and non-hyphen characters
		groupID := strings.Map(func(r rune) rune {
			if (r >= '0' && r <= '9') || r == '-' {
				return r
			}
			return -1
		}, number)
		return groupID + "@g.us", nil
	}

	// Remove all non-numeric characters for phone numbers
	number = strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, number)

	if number == "" {
		return "", fmt.Errorf("invalid number format")
	}

	// Format MX (52) or AR (54) numbers
	number = formatMXOrARNumber(number)

	// Format BR (55) numbers
	number = formatBRNumber(number)

	// Add + prefix for international format
	if !strings.HasPrefix(number, "+") {
		number = "+" + number
	}

	return number + "@s.whatsapp.net", nil
}

// formatMXOrARNumber formats Mexican (52) or Argentine (54) numbers
func formatMXOrARNumber(jid string) string {
	if len(jid) < 2 {
		return jid
	}

	countryCode := jid[:2]

	// Check if it's MX (52) or AR (54)
	if countryCode == "52" && len(jid) == 13 {
		// Mexico: remove 2 digits (positions 2-3)
		// 5215551234567 -> 52 + 551234567 = 52551234567
		return countryCode + jid[4:]
	} else if countryCode == "54" && len(jid) == 13 {
		// Argentina: remove 1 digit (position 2)
		// 5411123456789 -> 54 + 11123456789 = 5411123456789
		return countryCode + jid[3:]
	}

	return jid
}

// formatBRNumber formats Brazilian (55) numbers according to the mobile number rules
func formatBRNumber(jid string) string {
	// Only process if it's exactly 13 digits and starts with "55"
	if len(jid) != 13 || !strings.HasPrefix(jid, "55") {
		return jid
	}

	// Extract DDD (area code) - should be between 11-99 for Brazil
	ddd := jid[2:4]

	// Convert DDD to integer for validation
	dddNum, err := strconv.Atoi(ddd)
	if err != nil {
		return jid
	}

	// Brazilian DDD codes are between 11-99, if it's outside this range, it's not Brazil
	if dddNum < 11 || dddNum > 99 {
		return jid
	}

	// Extract the first digit after DDD
	if len(jid) < 6 {
		return jid
	}

	firstDigit := jid[4:5]
	firstDigitNum, err := strconv.Atoi(firstDigit)
	if err != nil {
		return jid
	}

	// Check if it's a mobile number (9 prefix) and DDD >= 31
	if firstDigitNum >= 7 && dddNum >= 31 {
		// Remove the 9 prefix for mobile numbers with DDD >= 31
		return jid[:4] + jid[5:]
	}

	// Keep the number as is (landline or special case)
	return jid
}

// ParseJID parses a number string into a WhatsApp JID with validation
func ParseJID(arg string) (whatsmeow_types.JID, bool) {
	if arg == "" {
		return whatsmeow_types.NewJID("", whatsmeow_types.DefaultUserServer), false
	}

	// Use CreateJID for consistent formatting
	jidString, err := CreateJID(arg)
	if err != nil {
		logger.LogWarn("Failed to create JID: %s", err.Error())
		return whatsmeow_types.NewJID("", whatsmeow_types.DefaultUserServer), false
	}

	// Parse the formatted JID
	recipient, err := whatsmeow_types.ParseJID(jidString)
	if err != nil {
		logger.LogWarn("Invalid JID: %s", err.Error())
		return recipient, false
	}

	if recipient.User == "" && !strings.Contains(jidString, "@broadcast") {
		logger.LogError("Invalid JID. No user specified: %s", jidString)
		return recipient, false
	}

	return recipient, true
}

func CreateHTTPProxy(httpHost, httpPort, user, password string) (func(*http.Request) (*url.URL, error), error) {
	address := fmt.Sprintf("http://%s:%s@%s:%s", user, password, httpHost, httpPort)

	parsed, err := url.Parse(address)
	if err != nil {
		return nil, err
	}

	return http.ProxyURL(parsed), nil
}

func CreateSocks5Proxy(socks5Host, socks5Port, user, password string) (proxy.Dialer, error) {
	auth := &proxy.Auth{
		User:     user,
		Password: password,
	}

	dialer, err := proxy.SOCKS5("tcp", fmt.Sprintf("%s:%s", socks5Host, socks5Port), auth, proxy.Direct)
	if err != nil {
		return nil, err
	}

	return dialer, nil

	// return func(req *http.Request) (*url.URL, error) {
	// 	host := req.URL.Host
	// 	if !strings.Contains(host, ":") {
	// 		host = fmt.Sprintf("%s:443", host) // Adiciona porta padrão 443 se não especificada
	// 	}
	// 	conn, err := dialer.Dial("tcp", host)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	defer conn.Close()

	// 	return nil, nil
	// }, nil
}

// NormalizeProxyProtocol returns the proxy protocol normalized to one of
// http/https/socks5. If not supplied, it is inferred from the port — ports
// 1080, 2080, and 42000-43000 map to socks5; everything else defaults to http.
func NormalizeProxyProtocol(protocol, port string) string {
	normalized := strings.ToLower(strings.TrimSpace(protocol))

	switch normalized {
	case "socks":
		return "socks5"
	case "http", "https", "socks5":
		return normalized
	}

	switch strings.TrimSpace(port) {
	case "1080", "2080":
		return "socks5"
	}

	portNum, err := strconv.Atoi(strings.TrimSpace(port))
	if err == nil && portNum >= 42000 && portNum <= 43000 {
		return "socks5"
	}

	return "http"
}

// BuildProxyAddress builds a proxy URL string suitable for whatsmeow's
// client.SetProxyAddress — it supports http, https, and socks5 with optional
// basic auth credentials.
func BuildProxyAddress(protocol, host, port, user, password string) (string, error) {
	if strings.TrimSpace(host) == "" {
		return "", fmt.Errorf("proxy host is required")
	}

	if strings.TrimSpace(port) == "" {
		return "", fmt.Errorf("proxy port is required")
	}

	normalizedProtocol := NormalizeProxyProtocol(protocol, port)

	if normalizedProtocol != "http" && normalizedProtocol != "https" && normalizedProtocol != "socks5" {
		return "", fmt.Errorf("unsupported proxy protocol %q", protocol)
	}

	proxyURL := &url.URL{
		Scheme: normalizedProtocol,
		Host:   net.JoinHostPort(strings.TrimSpace(host), strings.TrimSpace(port)),
	}

	if user != "" {
		if password != "" {
			proxyURL.User = url.UserPassword(user, password)
		} else {
			proxyURL.User = url.User(user)
		}
	}

	return proxyURL.String(), nil
}

func UpdateUserInfo(values interface{}, field string, value string) interface{} {
	v, ok := values.(Values)
	if !ok {
		logger.LogError("Failed to cast values to Values type")
		return values
	}

	logger.LogDebug("User info updated field: %s value: %s", field, value)
	v.m[field] = value
	return v
}

func TimestampToUnixInt(timestamp string) (int64, error) {
	layout := "2006-01-02 15:04:05"

	t, err := time.Parse(layout, timestamp)
	if err != nil {
		return 0, err
	}

	unixTimestamp := t.Unix()

	return unixTimestamp, nil
}

func GenerateVC(data VCardStruct) string {
	result := `
BEGIN:VCARD
VERSION:3.0
FN:` + data.FullName + `
ORG:` + data.Organization + `;
TEL;type=CELL;type=VOICE;waid=` + data.Phone + `:` + data.Phone + `
END:VCARD`

	return result
}

func GetObject(message []byte, keyFind string) string {
	var messageMap map[string]interface{}
	err := json.Unmarshal(message, &messageMap)
	if err != nil {
		logger.LogError("failed to unmarshal message: %s", err)
		return ""
	}
	for key, value := range messageMap {
		if key == keyFind {
			if captionStr, ok := value.(string); ok {
				return captionStr
			}
		}

		if nestedMap, ok := value.(map[string]interface{}); ok {
			nestedMapBytes, err := json.Marshal(nestedMap)
			if err != nil {
				logger.LogError("failed to marshal nestedMap: %s", err)
				continue
			}
			if caption := GetObject(nestedMapBytes, keyFind); caption != "" {
				return caption
			}
		}
	}
	return ""
}

func WhatsAppGetUserOS() string {
	switch runtime.GOOS {
	case "windows":
		return "Windows"
	case "darwin":
		return "macOS"
	default:
		return "Linux"
	}
}

func WhatsAppGetUserAgent(agentType string) waCompanionReg.DeviceProps_PlatformType {
	switch strings.ToLower(agentType) {
	case "desktop":
		return waCompanionReg.DeviceProps_DESKTOP
	case "mac":
		return waCompanionReg.DeviceProps_CATALINA
	case "android":
		return waCompanionReg.DeviceProps_ANDROID_AMBIGUOUS
	case "android-phone":
		return waCompanionReg.DeviceProps_ANDROID_PHONE
	case "andorid-tablet":
		return waCompanionReg.DeviceProps_ANDROID_TABLET
	case "ios-phone":
		return waCompanionReg.DeviceProps_IOS_PHONE
	case "ios-catalyst":
		return waCompanionReg.DeviceProps_IOS_CATALYST
	case "ipad":
		return waCompanionReg.DeviceProps_IPAD
	case "wearos":
		return waCompanionReg.DeviceProps_WEAR_OS
	case "ie":
		return waCompanionReg.DeviceProps_IE
	case "edge":
		return waCompanionReg.DeviceProps_EDGE
	case "chrome":
		return waCompanionReg.DeviceProps_CHROME
	case "safari":
		return waCompanionReg.DeviceProps_SAFARI
	case "firefox":
		return waCompanionReg.DeviceProps_FIREFOX
	case "opera":
		return waCompanionReg.DeviceProps_OPERA
	case "uwp":
		return waCompanionReg.DeviceProps_UWP
	case "aloha":
		return waCompanionReg.DeviceProps_ALOHA
	case "tv-tcl":
		return waCompanionReg.DeviceProps_TCL_TV
	default:
		return waCompanionReg.DeviceProps_UNKNOWN
	}
}

func GetMessageType(waMsg *waE2E.Message) string {
	switch {
	case waMsg == nil:
		return "ignore"
	case waMsg.Conversation != nil, waMsg.ExtendedTextMessage != nil:
		return "text"
	case waMsg.ImageMessage != nil:
		return fmt.Sprintf("image %s", waMsg.GetImageMessage().GetMimetype())
	case waMsg.StickerMessage != nil:
		return fmt.Sprintf("sticker %s", waMsg.GetStickerMessage().GetMimetype())
	case waMsg.VideoMessage != nil:
		return fmt.Sprintf("video %s", waMsg.GetVideoMessage().GetMimetype())
	case waMsg.PtvMessage != nil:
		return fmt.Sprintf("round video %s", waMsg.GetPtvMessage().GetMimetype())
	case waMsg.AudioMessage != nil:
		return fmt.Sprintf("audio %s", waMsg.GetAudioMessage().GetMimetype())
	case waMsg.DocumentMessage != nil:
		return fmt.Sprintf("document %s", waMsg.GetDocumentMessage().GetMimetype())
	case waMsg.ContactMessage != nil:
		return "contact"
	case waMsg.ContactsArrayMessage != nil:
		return "contact array"
	case waMsg.LocationMessage != nil:
		return "location"
	case waMsg.LiveLocationMessage != nil:
		return "live location start"
	case waMsg.GroupInviteMessage != nil:
		return "group invite"
	case waMsg.GroupMentionedMessage != nil:
		return "group mention"
	case waMsg.ScheduledCallCreationMessage != nil:
		return "scheduled call create"
	case waMsg.ScheduledCallEditMessage != nil:
		return "scheduled call edit"
	case waMsg.ReactionMessage != nil:
		if waMsg.ReactionMessage.GetText() == "" {
			return "reaction remove"
		}
		return "reaction"
	case waMsg.EncReactionMessage != nil:
		return "encrypted reaction"
	case waMsg.PollCreationMessage != nil || waMsg.PollCreationMessageV2 != nil || waMsg.PollCreationMessageV3 != nil:
		return "poll create"
	case waMsg.PollUpdateMessage != nil:
		return "poll update"
	case waMsg.ProtocolMessage != nil:
		switch waMsg.GetProtocolMessage().GetType() {
		case waE2E.ProtocolMessage_REVOKE:
			if waMsg.GetProtocolMessage().GetKey() == nil {
				return "ignore"
			}
			return "revoke"
		case waE2E.ProtocolMessage_MESSAGE_EDIT:
			return "edit"
		case waE2E.ProtocolMessage_EPHEMERAL_SETTING:
			return "disappearing timer change"
		case waE2E.ProtocolMessage_APP_STATE_SYNC_KEY_SHARE,
			waE2E.ProtocolMessage_HISTORY_SYNC_NOTIFICATION,
			waE2E.ProtocolMessage_INITIAL_SECURITY_NOTIFICATION_SETTING_SYNC:
			return "ignore"
		default:
			return fmt.Sprintf("unknown_protocol_%d", waMsg.GetProtocolMessage().GetType())
		}
	case waMsg.ButtonsMessage != nil:
		return "buttons"
	case waMsg.ButtonsResponseMessage != nil:
		return "buttons response"
	case waMsg.TemplateMessage != nil:
		return "template"
	case waMsg.HighlyStructuredMessage != nil:
		return "highly structured template"
	case waMsg.TemplateButtonReplyMessage != nil:
		return "template button reply"
	case waMsg.InteractiveMessage != nil:
		return "interactive"
	case waMsg.GetInteractiveResponseMessage() != nil:
		return "interactive response"
	case waMsg.ListMessage != nil:
		return "list"
	case waMsg.ProductMessage != nil:
		return "product"
	case waMsg.ListResponseMessage != nil:
		return "list response"
	case waMsg.OrderMessage != nil:
		return "order"
	case waMsg.InvoiceMessage != nil:
		return "invoice"
	case waMsg.BotInvokeMessage != nil:
		return "bot invoke"
	case waMsg.EventMessage != nil:
		return "event"
	case waMsg.EventCoverImage != nil:
		return "event cover image"
	case waMsg.EncEventResponseMessage != nil:
		return "ignore" // these are ignored for now as they're not meant to be shown as new messages
		//return "encrypted event response"
	case waMsg.CommentMessage != nil:
		return "comment"
	case waMsg.EncCommentMessage != nil:
		return "encrypted comment"
	case waMsg.NewsletterAdminInviteMessage != nil:
		return "newsletter admin invite"
	case waMsg.SecretEncryptedMessage != nil:
		return "secret encrypted"
	case waMsg.PollResultSnapshotMessage != nil:
		return "poll result snapshot"
	case waMsg.MessageHistoryBundle != nil:
		return "message history bundle"
	case waMsg.RequestPhoneNumberMessage != nil:
		return "request phone number"
	case waMsg.KeepInChatMessage != nil:
		return "keep in chat"
	case waMsg.StatusMentionMessage != nil:
		return "status mention"
	case waMsg.StickerPackMessage != nil:
		return "sticker pack"
	case waMsg.AlbumMessage != nil:
		return "album" // or maybe these should be ignored?
	case waMsg.SendPaymentMessage != nil, waMsg.RequestPaymentMessage != nil,
		waMsg.DeclinePaymentRequestMessage != nil, waMsg.CancelPaymentRequestMessage != nil,
		waMsg.PaymentInviteMessage != nil:
		return "payment"
	case waMsg.Call != nil:
		return "call"
	case waMsg.Chat != nil:
		return "chat"
	case waMsg.PlaceholderMessage != nil:
		return "placeholder"
	case waMsg.SenderKeyDistributionMessage != nil, waMsg.StickerSyncRmrMessage != nil:
		return "ignore"
	default:
		return "unknown"
	}
}

func GetStringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// PrepareNumbersForWhatsAppCheck prepares phone numbers for IsOnWhatsApp call
// based on formatJid flag. This centralizes the logic used by both CheckUser and SendText.
func PrepareNumbersForWhatsAppCheck(numbers []string, formatJid *bool) ([]string, error) {
	// Default formatJid to true if not specified
	shouldFormat := true
	if formatJid != nil {
		shouldFormat = *formatJid
	}

	var phoneNumbers []string

	if shouldFormat {
		// Normalize numbers using CreateJID for consistent formatting
		for _, number := range numbers {
			// First, extract the raw number if it's already a JID
			rawNumber := number
			if strings.Contains(number, "@s.whatsapp.net") {
				rawNumber = strings.Split(number, "@")[0]
			}

			// Use CreateJID to normalize the raw number format
			normalizedJID, err := CreateJID(rawNumber)
			if err != nil {
				// Continue with original number if normalization fails
				phoneNumbers = append(phoneNumbers, number)
				continue
			}

			// Extract the phone number part from the JID for IsOnWhatsApp call
			// e.g., "+5511999999999@s.whatsapp.net" -> "+5511999999999" (keep + for IsOnWhatsApp)
			if strings.Contains(normalizedJID, "@s.whatsapp.net") {
				phoneNumber := strings.Split(normalizedJID, "@")[0]
				phoneNumbers = append(phoneNumbers, phoneNumber)
			} else if strings.Contains(normalizedJID, "@g.us") || strings.Contains(normalizedJID, "@broadcast") || strings.Contains(normalizedJID, "@lid") {
				// For groups, broadcasts, and LIDs, use the full JID
				phoneNumbers = append(phoneNumbers, normalizedJID)
			} else {
				phoneNumbers = append(phoneNumbers, normalizedJID)
			}
		}
	} else {
		// Use numbers exactly as received (raw format)
		phoneNumbers = append(phoneNumbers, numbers...)
	}

	return phoneNumbers, nil
}

// PrepareNumberForWhatsAppCheck prepares a single phone number for IsOnWhatsApp call
// This is used by SendText which works with single numbers
func PrepareNumberForWhatsAppCheck(phone string, formatJid bool) (string, error) {
	formatJidPtr := &formatJid
	numbers, err := PrepareNumbersForWhatsAppCheck([]string{phone}, formatJidPtr)
	if err != nil {
		return "", err
	}
	if len(numbers) == 0 {
		return "", fmt.Errorf("no valid number processed")
	}
	return numbers[0], nil
}

// CanonicalJID normalizes a JID user part by stripping the leading "+" that
// some clients include, so lookups and comparisons match the stored form.
func CanonicalJID(jid whatsmeow_types.JID) whatsmeow_types.JID {
	jid.User = strings.TrimPrefix(jid.User, "+")
	return jid
}
