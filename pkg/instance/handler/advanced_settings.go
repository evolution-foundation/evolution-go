package instance_handler

import (
	"bytes"
	"errors"
	"net/http"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// bindInstanceSettingsJSON keeps Gin's validation while rejecting a null root.
// Callers bind into concrete structs so null cannot reach Gin's validator as a
// nil pointer. Null fields within the object retain partial-update semantics.
func bindInstanceSettingsJSON(c *gin.Context, data interface{}) error {
	if err := c.ShouldBindBodyWithJSON(data); err != nil {
		return err
	}
	value, _ := c.Get(gin.BodyBytesKey)
	body, _ := value.([]byte)
	if bytes.Equal(bytes.TrimSpace(body), []byte("null")) {
		return errors.New("request body must be an object")
	}
	return nil
}

// advancedSettingsInstanceID scopes path-based settings to the authenticated
// instance. Return its stored ID so equivalent UUID spellings use the same row.
func advancedSettingsInstanceID(c *gin.Context) (string, bool) {
	pathID := c.Param("instanceId")
	id, err := uuid.Parse(pathID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid instanceId"})
		return "", false
	}
	value, _ := c.Get("instance")
	instance, ok := value.(*instance_model.Instance)
	if !ok || instance == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not authorized"})
		return "", false
	}
	authenticatedID, err := uuid.Parse(instance.Id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid authenticated instance"})
		return "", false
	}
	if id != authenticatedID {
		c.JSON(http.StatusForbidden, gin.H{"error": "instanceId does not match authenticated instance"})
		return "", false
	}
	return instance.Id, true
}

func writeInstanceSettingsError(c *gin.Context, err error) {
	if errors.Is(err, instance_model.ErrInstanceNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to access instance settings"})
}
