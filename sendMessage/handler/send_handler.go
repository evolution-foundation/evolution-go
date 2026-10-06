package send_handler

import (
	"encoding/base64"
	"io"
	"net/http"
	"strconv"
	"strings"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	send_service "github.com/evolution-foundation/evolution-go/pkg/sendMessage/service"
	"github.com/gin-gonic/gin"
)

type SendHandler interface {
	SendText(ctx *gin.Context)
	SendLink(ctx *gin.Context)
	SendMedia(ctx *gin.Context)
	SendPoll(ctx *gin.Context)
	SendSticker(ctx *gin.Context)
	SendLocation(ctx *gin.Context)
	SendContact(ctx *gin.Context)
	SendButton(ctx *gin.Context)
	SendList(ctx *gin.Context)
	SendCarousel(ctx *gin.Context)
	SendStatusText(ctx *gin.Context)
	SendStatusMedia(ctx *gin.Context)
	SendForward(ctx *gin.Context)
}

type sendHandler struct {
	sendMessageService send_service.SendService
}

// Send a text message
// @Summary Send a text message
// @Description Send a text message
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body send_service.TextStruct true "Message data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /send/text [post]
func (s *sendHandler) SendText(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *send_service.TextStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Number == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	if data.Text == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "message body is required"})
		return
	}

	message, err := s.sendMessageService.SendText(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
}

// Forward a message
// @Summary Forward a message
// @Description Forward an existing message to another chat, flagged as forwarded. The `message` field must contain the full waE2E.Message payload of the original message (e.g. `data.Message` as delivered by the webhook). Media is forwarded by reference, without re-upload.
// @Tags Message
// @Accept json
// @Produce json
// @Param message body send_service.ForwardStruct true "Forward data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /message/forward [post]
func (s *sendHandler) SendForward(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *send_service.ForwardStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Number == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	if data.Message == nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "message payload is required"})
		return
	}

	message, err := s.sendMessageService.SendForward(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
}

// Send a link message
// @Summary Send a link message
// @Description Send a link message
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body send_service.LinkStruct true "Message data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /send/link [post]
func (s *sendHandler) SendLink(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *send_service.LinkStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Number == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	if data.Text == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "message body is required"})
		return
	}

	message, err := s.sendMessageService.SendLink(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
}

// Send a media message
// @Summary Send a media message
// @Description Send a media message
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body send_service.MediaStruct true "Message data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /send/media [post]
func (s *sendHandler) SendMedia(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	contentType := ctx.ContentType()

	var data *send_service.MediaStruct

	if strings.HasPrefix(contentType, "multipart/form-data") {
		// Handle form-data
		number := ctx.PostForm("number")
		if number == "" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
			return
		}

		mediaType := ctx.PostForm("type")
		if mediaType == "" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "media type is required"})
			return
		}

		caption := ctx.PostForm("caption")
		filename := ctx.PostForm("filename")
		id := ctx.PostForm("id")
		delayStr := ctx.PostForm("delay")
		delay := int32(0)
		if delayStr != "" {
			delay64, err := strconv.ParseInt(delayStr, 10, 32)
			if err != nil {
				ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid delay"})
				return
			}
			delay = int32(delay64)
		}

		mentionAll := ctx.PostForm("mentionAll") == "true"

		var mentionedJID []string
		// Accept multiple values (mentionedJid=x&mentionedJid=y) or a single
		// comma-separated string (mentionedJid=x,y).
		for _, raw := range ctx.PostFormArray("mentionedJid") {
			for _, v := range strings.Split(raw, ",") {
				if trimmed := strings.TrimSpace(v); trimmed != "" {
					mentionedJID = append(mentionedJID, trimmed)
				}
			}
		}

		var quoted send_service.QuotedStruct
		quoted.MessageID = ctx.PostForm("quoted.messageId")
		quoted.Participant = ctx.PostForm("quoted.participant")

		// Get file
		file, err := ctx.FormFile("file")
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
			return
		}

		// Open file
		fileData, err := file.Open()
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "cannot open file"})
			return
		}
		defer fileData.Close()
		fileBytes, err := io.ReadAll(fileData)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "cannot read file"})
			return
		}

		// Create MediaStruct
		data = &send_service.MediaStruct{
			Number:       number,
			Type:         mediaType,
			Caption:      caption,
			Filename:     filename,
			Id:           id,
			Delay:        delay,
			MentionAll:   mentionAll,
			MentionedJID: mentionedJID,
			Quoted:       quoted,
		}

		// Pass fileBytes to the send service
		message, err := s.sendMessageService.SendMediaFile(data, fileBytes, instance)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})

	} else {

		err := ctx.ShouldBindBodyWithJSON(&data)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if data.Number == "" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
			return
		}

		if data.Url == "" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "URL is required"})
			return
		}

		if data.Type == "" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "media type is required"})
			return
		}

		var message *send_service.MessageSendStruct

		if !strings.HasPrefix(data.Url, "http://") && !strings.HasPrefix(data.Url, "https://") {
			// Treat as base64-encoded media
			fileBytes, err := base64.StdEncoding.DecodeString(data.Url)
			if err != nil {
				ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid base64 encoding"})
				return
			}
			message, err = s.sendMessageService.SendMediaFile(data, fileBytes, instance)
			if err != nil {
				ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		} else {
			message, err = s.sendMessageService.SendMediaUrl(data, instance)
			if err != nil {
				ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}

		ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
	}
}

// Send a poll message
// @Summary Send a poll message
// @Description Send a poll message
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body send_service.PollStruct true "Message data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /send/poll [post]
func (s *sendHandler) SendPoll(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *send_service.PollStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Number == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	if data.Question == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "question is required"})
		return
	}

	if len(data.Options) < 2 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "minimum 2 options are required"})
		return
	}

	message, err := s.sendMessageService.SendPoll(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
}

// Send a sticker message
// @Summary Send a sticker message
// @Description Send a sticker message
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body send_service.StickerStruct true "Message data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /send/sticker [post]
func (s *sendHandler) SendSticker(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *send_service.StickerStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Number == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	if data.Sticker == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "sticker is required"})
		return
	}

	message, err := s.sendMessageService.SendSticker(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
}

// Send a location message
// @Summary Send a location message
// @Description Send a location message
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body send_service.LocationStruct true "Message data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /send/location [post]
func (s *sendHandler) SendLocation(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *send_service.LocationStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Number == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	if data.Latitude == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "latitude is required"})
		return
	}

	if data.Longitude == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "longitude is required"})
		return
	}

	if data.Address == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "address is required"})
		return
	}

	if data.Name == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	message, err := s.sendMessageService.SendLocation(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
}

// Send a contact message
// @Summary Send a contact message
// @Description Send a contact message
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body send_service.ContactStruct true "Message data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /send/contact [post]
func (s *sendHandler) SendContact(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *send_service.ContactStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Number == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	if data.Vcard.Phone == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "contact phone number is required"})
		return
	}

	if data.Vcard.FullName == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "contact full name is required"})
		return
	}

	message, err := s.sendMessageService.SendContact(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
}

// Send a button message
// @Summary Send a button message
// @Description Send an interactive message with buttons. Each button has a `type`: `reply`, `copy`, `url`, `call` or `pix`.
// @Description
// @Description Combination rules enforced by the server:
// @Description   - Up to 3 `reply` buttons per message.
// @Description   - `reply` buttons cannot be mixed with any other type.
// @Description   - `pix` button must be sent ALONE (no other button in the same message).
// @Description
// @Description WhatsApp client rendering quirks (NOT enforced by the server, but verified in the field):
// @Description   - WhatsApp Web: only `reply`-only messages (up to 3) OR CTAs grouped together (`copy` + `url` + `call`) render correctly.
// @Description   - Do NOT mix `reply` with CTA buttons (`copy`/`url`/`call`) — the message will not appear on WhatsApp Web.
// @Description
// @Description Required body fields: `number`, `title`, `description`, `footer`, `buttons`.
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body send_service.ButtonStruct true "Message data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /send/button [post]
func (s *sendHandler) SendButton(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *send_service.ButtonStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Number == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	if data.Title == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}

	if data.Description == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "description is required"})
		return
	}

	if data.Footer == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "footer is required"})
		return
	}

	message, err := s.sendMessageService.SendButton(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
}

// Send a list message
// @Summary Send a list message
// @Description Send an interactive list message (single-select) rendered as a tappable menu.
// @Description
// @Description Required body fields: `number`, `title`, `description`, `footerText`, `buttonText`, `sections`.
// @Description Each section must contain one or more `rows`. When `rowId` is omitted, the server generates a fallback ID.
// @Description When `buttonText` is empty, the server falls back to "Ver Menu".
// @Description
// @Description Uses legacy `ListMessage` format (no ViewOnceMessage wrapper) so it renders on iOS, Android and WhatsApp Web.
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body send_service.ListStruct true "Message data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /send/list [post]
func (s *sendHandler) SendList(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *send_service.ListStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Number == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	if data.Title == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}

	if data.Description == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "description is required"})
		return
	}

	if data.FooterText == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "footer is required"})
		return
	}

	if data.ButtonText == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "button text is required"})
		return
	}

	message, err := s.sendMessageService.SendList(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
}

// Send a carousel message
// @Summary Send a carousel message
// @Description Send an interactive carousel (multiple swipeable cards). Each card carries its own image or video, body and optional buttons.
// @Description
// @Description Card button `type` accepted values (case-insensitive, uppercased internally): `REPLY` (default), `URL`, `CALL`, `COPY`.
// @Description The `PIX` button type is NOT supported in carousel cards — use `/send/button` for PIX.
// @Description
// @Description IMPORTANT — `CarouselButtonStruct` is different from the flat button used in `/send/button`:
// @Description   - URL button: put the link in the `id` field (NOT in a `url` field).
// @Description   - CALL button: put the phone number in the `id` field (NOT in a `phoneNumber` field).
// @Description   - COPY button: put the code to be copied in `copyCode`.
// @Description   - REPLY button: put the payload/callback ID in `id`.
// @Description
// @Description Per-card combination rules (NOT enforced by the server, but verified in the field):
// @Description   - Same WhatsApp Web quirk as `/send/button`: avoid mixing REPLY with CTA buttons (URL/CALL/COPY) in the same card — mixed sets do not render on Web.
// @Description   - Stick to either "only REPLY" or "only CTAs grouped together" per card.
// @Description
// @Description Required body fields: `number`, `cards` (at least one). Each card requires `header` + `body`.
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body send_service.CarouselStruct true "Message data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /send/carousel [post]
func (s *sendHandler) SendCarousel(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *send_service.CarouselStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Number == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	if len(data.Cards) == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "at least one card is required"})
		return
	}

	message, err := s.sendMessageService.SendCarousel(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
}

// Send a text status message
// @Summary Send a WhatsApp text status
// @Description Send a WhatsApp text status to status@broadcast
// @Tags Send Message
// @Accept json
// @Produce json
// @Param message body send_service.StatusTextStruct true "Status text data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /send/status/text [post]
func (s *sendHandler) SendStatusText(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	data := new(send_service.StatusTextStruct)
	err := ctx.ShouldBindBodyWithJSON(data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Text == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "text is required"})
		return
	}

	message, err := s.sendMessageService.SendStatusText(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
}

// Send a media status message (image or video)
// @Summary Send a WhatsApp media status (image/video)
// @Description Send an image or video status to status@broadcast. Supports JSON (URL) or multipart/form-data (file upload)
// @Tags Send Message
// @Accept json, multipart/form-data
// @Produce json
// @Param type formData string true "Media type: image or video"
// @Param file formData file false "Media file (for multipart upload)"
// @Param url formData string false "Media URL (for JSON upload)"
// @Param caption formData string false "Caption for the media"
// @Param id formData string false "Custom message ID"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /send/status/media [post]
func (s *sendHandler) SendStatusMedia(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	contentType := ctx.ContentType()

	data := new(send_service.StatusMediaStruct)

	if strings.HasPrefix(contentType, "multipart/form-data") {
		mediaType := ctx.PostForm("type")
		if mediaType == "" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "media type is required"})
			return
		}

		if mediaType != "image" && mediaType != "video" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "type must be 'image' or 'video'"})
			return
		}

		caption := ctx.PostForm("caption")
		id := ctx.PostForm("id")

		file, err := ctx.FormFile("file")
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
			return
		}

		fileData, err := file.Open()
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "cannot open file"})
			return
		}
		defer fileData.Close()
		fileBytes, err := io.ReadAll(fileData)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "cannot read file"})
			return
		}

		data = &send_service.StatusMediaStruct{
			Type:    mediaType,
			Caption: caption,
			Id:      id,
		}

		message, err := s.sendMessageService.SendStatusMediaFile(data, fileBytes, instance)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
		return
	}

	err := ctx.ShouldBindBodyWithJSON(data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Url == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "url is required"})
		return
	}

	if data.Type != "image" && data.Type != "video" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "type must be 'image' or 'video'"})
		return
	}

	message, err := s.sendMessageService.SendStatusMediaUrl(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": message})
}

func NewSendHandler(
	sendMessageService send_service.SendService,
) SendHandler {
	return &sendHandler{
		sendMessageService: sendMessageService,
	}
}
