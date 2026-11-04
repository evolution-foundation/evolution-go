package auth_middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/evolution-foundation/evolution-go/pkg/utils"
	"github.com/gin-gonic/gin"
	"github.com/gomessguii/logger"
)

// JIDValidationMiddleware validates JID parameters in request bodies
type JIDValidationMiddleware struct{}

// NewJIDValidationMiddleware creates a new JID validation middleware
func NewJIDValidationMiddleware() *JIDValidationMiddleware {
	return &JIDValidationMiddleware{}
}

// ValidateJIDFields validates and normalizes JID fields in request body
func (m *JIDValidationMiddleware) ValidateJIDFields(fieldNames ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only process JSON requests
		contentType := c.ContentType()
		if !strings.Contains(contentType, "application/json") {
			// For multipart/form-data, validate form fields
			if strings.Contains(contentType, "multipart/form-data") {
				m.validateFormFields(c, fieldNames...)
				return
			}
			c.Next()
			return
		}

		// Read the request body
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
			c.Abort()
			return
		}

		// Restore the request body for downstream handlers
		c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

		// Parse JSON
		var requestData map[string]interface{}
		if err := json.Unmarshal(body, &requestData); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON format"})
			c.Abort()
			return
		}

		// Validate and normalize JID fields
		modified := false
		for _, fieldName := range fieldNames {
			if value, exists := requestData[fieldName]; exists {
				if strValue, ok := value.(string); ok && strValue != "" {
					// Validate and normalize the JID
					normalizedJID, err := utils.CreateJID(strValue)
					if err != nil {
						c.JSON(http.StatusBadRequest, gin.H{
							"error": fmt.Sprintf("Invalid %s format: %s", fieldName, err.Error()),
						})
						c.Abort()
						return
					}

					// Update the value if it was normalized
					if normalizedJID != strValue {
						requestData[fieldName] = normalizedJID
						modified = true
						logger.LogDebug("Normalized identity field %s", fieldName)
					}
				} else if strValue == "" {
					c.JSON(http.StatusBadRequest, gin.H{
						"error": fmt.Sprintf("%s is required and cannot be empty", fieldName),
					})
					c.Abort()
					return
				}
			}
		}

		// If we modified the request, update the body
		if modified {
			newBody, err := json.Marshal(requestData)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process request"})
				c.Abort()
				return
			}
			c.Request.Body = io.NopCloser(bytes.NewBuffer(newBody))
		}

		c.Next()
	}
}

// validateFormFields validates JID fields in multipart form data
func (m *JIDValidationMiddleware) validateFormFields(c *gin.Context, fieldNames ...string) {
	for _, fieldName := range fieldNames {
		value := c.PostForm(fieldName)
		if value != "" {
			// Validate the JID format
			_, err := utils.CreateJID(value)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": fmt.Sprintf("Invalid %s format: %s", fieldName, err.Error()),
				})
				c.Abort()
				return
			}
		} else if fieldName == "number" { // number is typically required
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("%s is required and cannot be empty", fieldName),
			})
			c.Abort()
			return
		}
	}
	c.Next()
}

// ValidateNumberField is a convenience method for the common "number" field
// It handles both single strings and arrays of strings
func (m *JIDValidationMiddleware) ValidateNumberField() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only process JSON requests
		contentType := c.ContentType()
		if !strings.Contains(contentType, "application/json") {
			// For multipart/form-data, validate form fields
			if strings.Contains(contentType, "multipart/form-data") {
				m.validateFormFields(c, "number")
				return
			}
			c.Next()
			return
		}

		// Read the request body
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
			c.Abort()
			return
		}

		// Restore the request body for downstream handlers
		c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

		// Parse JSON
		var requestData map[string]interface{}
		if err := json.Unmarshal(body, &requestData); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON format"})
			c.Abort()
			return
		}

		// Validate and normalize number field (can be string or array)
		modified := false
		if value, exists := requestData["number"]; exists {
			// Handle array of strings
			if arrayValue, ok := value.([]interface{}); ok {
				if len(arrayValue) == 0 {
					c.JSON(http.StatusBadRequest, gin.H{
						"error": "number array cannot be empty",
					})
					c.Abort()
					return
				}

				for i, item := range arrayValue {
					if strValue, ok := item.(string); ok && strValue != "" {
						normalizedJID, err := utils.CreateJID(strValue)
						if err != nil {
							c.JSON(http.StatusBadRequest, gin.H{
								"error": fmt.Sprintf("Invalid number[%d] format: %s", i, err.Error()),
							})
							c.Abort()
							return
						}

						if normalizedJID != strValue {
							arrayValue[i] = normalizedJID
							modified = true
							logger.LogDebug("Normalized identity at index %d", i)
						}
					} else if strValue == "" {
						c.JSON(http.StatusBadRequest, gin.H{
							"error": fmt.Sprintf("number[%d] cannot be empty", i),
						})
						c.Abort()
						return
					}
				}
			} else if strValue, ok := value.(string); ok {
				// Handle single string
				if strValue == "" {
					c.JSON(http.StatusBadRequest, gin.H{
						"error": "number is required and cannot be empty",
					})
					c.Abort()
					return
				}

				normalizedJID, err := utils.CreateJID(strValue)
				if err != nil {
					c.JSON(http.StatusBadRequest, gin.H{
						"error": fmt.Sprintf("Invalid number format: %s", err.Error()),
					})
					c.Abort()
					return
				}

				if normalizedJID != strValue {
					requestData["number"] = normalizedJID
					modified = true
					logger.LogDebug("Normalized recipient identity")
				}
			} else {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "number must be a string or array of strings",
				})
				c.Abort()
				return
			}
		}

		// If we modified the request, update the body
		if modified {
			newBody, err := json.Marshal(requestData)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process request"})
				c.Abort()
				return
			}
			c.Request.Body = io.NopCloser(bytes.NewBuffer(newBody))
		}

		c.Next()
	}
}

// ValidateMultipleNumbers validates multiple number fields (for arrays or multiple contacts)
func (m *JIDValidationMiddleware) ValidateMultipleNumbers(fieldName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only process JSON requests
		contentType := c.ContentType()
		if !strings.Contains(contentType, "application/json") {
			c.Next()
			return
		}

		// Read the request body
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
			c.Abort()
			return
		}

		// Restore the request body for downstream handlers
		c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

		// Parse JSON
		var requestData map[string]interface{}
		if err := json.Unmarshal(body, &requestData); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON format"})
			c.Abort()
			return
		}

		// Validate array of numbers
		if value, exists := requestData[fieldName]; exists {
			modified := false

			// Handle array of strings
			if arrayValue, ok := value.([]interface{}); ok {
				for i, item := range arrayValue {
					if strValue, ok := item.(string); ok && strValue != "" {
						normalizedJID, err := utils.CreateJID(strValue)
						if err != nil {
							c.JSON(http.StatusBadRequest, gin.H{
								"error": fmt.Sprintf("Invalid %s[%d] format: %s", fieldName, i, err.Error()),
							})
							c.Abort()
							return
						}

						if normalizedJID != strValue {
							arrayValue[i] = normalizedJID
							modified = true
						}
					}
				}
			}

			// If we modified the request, update the body
			if modified {
				newBody, err := json.Marshal(requestData)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process request"})
					c.Abort()
					return
				}
				c.Request.Body = io.NopCloser(bytes.NewBuffer(newBody))
			}
		}

		c.Next()
	}
}

// ValidateNumberFieldWithFormatJid validates number field but respects FormatJid parameter
// When FormatJid is true (default), numbers are normalized to full JID format
// When FormatJid is false, numbers are kept as received (raw format)
func (m *JIDValidationMiddleware) ValidateNumberFieldWithFormatJid() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only process JSON requests
		contentType := c.ContentType()
		if !strings.Contains(contentType, "application/json") {
			c.Next()
			return
		}

		// Read the request body
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
			c.Abort()
			return
		}

		// Restore the request body for downstream handlers
		c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

		// Parse JSON
		var requestData map[string]interface{}
		if err := json.Unmarshal(body, &requestData); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON format"})
			c.Abort()
			return
		}

		// Check FormatJid parameter (default is true)
		formatJid := true
		if formatJidValue, exists := requestData["formatJid"]; exists {
			if formatJidBool, ok := formatJidValue.(bool); ok {
				formatJid = formatJidBool
			}
		}

		// Validate and optionally normalize number field based on FormatJid
		modified := false
		if value, exists := requestData["number"]; exists {
			// Handle array of strings
			if arrayValue, ok := value.([]interface{}); ok {
				if len(arrayValue) == 0 {
					c.JSON(http.StatusBadRequest, gin.H{
						"error": "number array cannot be empty",
					})
					c.Abort()
					return
				}

				for i, item := range arrayValue {
					if strValue, ok := item.(string); ok && strValue != "" {
						// Only validate and normalize if FormatJid is true
						if formatJid {
							normalizedJID, err := utils.CreateJID(strValue)
							if err != nil {
								c.JSON(http.StatusBadRequest, gin.H{
									"error": fmt.Sprintf("Invalid number[%d] format: %s", i, err.Error()),
								})
								c.Abort()
								return
							}

							if normalizedJID != strValue {
								arrayValue[i] = normalizedJID
								modified = true
								logger.LogDebug("Normalized identity at index %d", i)
							}
						}
						// When formatJid is false, we accept numbers as received without validation
					} else if strValue == "" {
						c.JSON(http.StatusBadRequest, gin.H{
							"error": fmt.Sprintf("number[%d] cannot be empty", i),
						})
						c.Abort()
						return
					}
				}
			} else if strValue, ok := value.(string); ok {
				// Handle single string
				if strValue == "" {
					c.JSON(http.StatusBadRequest, gin.H{
						"error": "number is required and cannot be empty",
					})
					c.Abort()
					return
				}

				// Only validate and normalize if FormatJid is true
				if formatJid {
					normalizedJID, err := utils.CreateJID(strValue)
					if err != nil {
						c.JSON(http.StatusBadRequest, gin.H{
							"error": fmt.Sprintf("Invalid number format: %s", err.Error()),
						})
						c.Abort()
						return
					}

					if normalizedJID != strValue {
						requestData["number"] = normalizedJID
						modified = true
						logger.LogDebug("Normalized recipient identity")
					}
				}
				// When formatJid is false, we accept numbers as received without validation
			} else {
				c.JSON(http.StatusBadRequest, gin.H{
					"error": "number must be a string or array of strings",
				})
				c.Abort()
				return
			}
		}

		// If we modified the request, update the body
		if modified {
			newBody, err := json.Marshal(requestData)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process request"})
				c.Abort()
				return
			}
			c.Request.Body = io.NopCloser(bytes.NewBuffer(newBody))
		}

		c.Next()
	}
}

// ValidateContactFields validates contact-specific fields that may contain phone numbers
func (m *JIDValidationMiddleware) ValidateContactFields() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only process JSON requests
		contentType := c.ContentType()
		if !strings.Contains(contentType, "application/json") {
			c.Next()
			return
		}

		// Read the request body
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
			c.Abort()
			return
		}

		// Restore the request body for downstream handlers
		c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

		// Parse JSON
		var requestData map[string]interface{}
		if err := json.Unmarshal(body, &requestData); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON format"})
			c.Abort()
			return
		}

		modified := false

		// Validate main number field
		if value, exists := requestData["number"]; exists {
			if strValue, ok := value.(string); ok && strValue != "" {
				normalizedJID, err := utils.CreateJID(strValue)
				if err != nil {
					c.JSON(http.StatusBadRequest, gin.H{
						"error": fmt.Sprintf("Invalid number format: %s", err.Error()),
					})
					c.Abort()
					return
				}

				if normalizedJID != strValue {
					requestData["number"] = normalizedJID
					modified = true
				}
			}
		}

		// Validate vcard phone field if present
		if vcardValue, exists := requestData["vcard"]; exists {
			if vcardMap, ok := vcardValue.(map[string]interface{}); ok {
				if phoneValue, phoneExists := vcardMap["phone"]; phoneExists {
					if phoneStr, ok := phoneValue.(string); ok && phoneStr != "" {
						// For vcard phone, we just validate format but don't necessarily convert to JID
						_, err := utils.CreateJID(phoneStr)
						if err != nil {
							c.JSON(http.StatusBadRequest, gin.H{
								"error": fmt.Sprintf("Invalid vcard phone format: %s", err.Error()),
							})
							c.Abort()
							return
						}
					}
				}
			}
		}

		// If we modified the request, update the body
		if modified {
			newBody, err := json.Marshal(requestData)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process request"})
				c.Abort()
				return
			}
			c.Request.Body = io.NopCloser(bytes.NewBuffer(newBody))
		}

		c.Next()
	}
}
