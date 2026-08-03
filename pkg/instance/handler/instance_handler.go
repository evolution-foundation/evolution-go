package instance_handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	config "github.com/evolution-foundation/evolution-go/pkg/config"
	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	instance_service "github.com/evolution-foundation/evolution-go/pkg/instance/service"
	"github.com/evolution-foundation/evolution-go/pkg/utils"
)

type InstanceHandler interface {
	Create(ctx *gin.Context)
	Connect(ctx *gin.Context)
	Reconnect(ctx *gin.Context)
	Disconnect(ctx *gin.Context)
	Logout(ctx *gin.Context)
	Delete(ctx *gin.Context)
	Status(ctx *gin.Context)
	Qr(ctx *gin.Context)
	All(ctx *gin.Context)
	Info(ctx *gin.Context)
	Pair(ctx *gin.Context)
	SetProxy(ctx *gin.Context)
	DeleteProxy(ctx *gin.Context)
	ForceReconnect(ctx *gin.Context)
	GetLogs(ctx *gin.Context)
	GetAdvancedSettings(ctx *gin.Context)
	UpdateAdvancedSettings(ctx *gin.Context)
}

type instanceHandler struct {
	config          *config.Config
	instanceService instance_service.InstanceService
}

// Create a new instance
// @Summary Create a new instance
// @Description Creates a new instance with the provided data including optional advanced settings
// @Tags Instance
// @Accept json
// @Produce json
// @Param instance body instance_service.CreateStruct true "Instance data with optional advanced settings"
// @Success 200 {object} gin.H "Instance created successfully"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /instance/create [post]
func (i *instanceHandler) Create(ctx *gin.Context) {
	var data *instance_service.CreateStruct
	err := ctx.ShouldBindJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Name == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	if data.Token == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "token is required"})
		return
	}

	if data.Proxy != nil {
		if data.Proxy.Port == "" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "proxy port is required"})
			return
		}

		if data.Proxy.Password == "" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "proxy password is required"})
			return
		}

		if data.Proxy.Username == "" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "proxy username is required"})
			return
		}

		if data.Proxy.Host == "" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "proxy host is required"})
			return
		}
	} else {
		if i.config.ProxyHost != "" && i.config.ProxyPort != "" && i.config.ProxyUsername != "" && i.config.ProxyPassword != "" {
			data.Proxy = &instance_service.ProxyConfig{
				Host:     i.config.ProxyHost,
				Port:     i.config.ProxyPort,
				Username: i.config.ProxyUsername,
				Password: i.config.ProxyPassword,
			}
		}
	}

	createdInstance, err := i.instanceService.Create(data)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": createdInstance})
}

// Connect to instance
// @Summary Connect to instance
// @Description Connect to instance with the provided data
// @Tags Instance
// @Accept json
// @Produce json
// @Param instance body instance_service.ConnectStruct true "Instance data"
// @Success 200 {object} gin.H "Instance connected successfully"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /instance/connect [post]
func (i *instanceHandler) Connect(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *instance_service.ConnectStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	instance, jid, eventString, err := i.instanceService.Connect(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Set("instance", instance)

	responseData := gin.H{
		"jid":         jid,
		"webhookUrl":  instance.Webhook,
		"eventString": eventString,
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": responseData})
}

// Reconnect to instance
// @Summary Reconnect to instance
// @Description Reconnect to instance
// @Tags Instance
// @Accept json
// @Produce json
// @Success 200 {object} gin.H "Instance reconnected successfully"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /instance/reconnect [post]
func (i *instanceHandler) Reconnect(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	err := i.instanceService.Reconnect(instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success"})
}

// Disconnect from instance
// @Summary Disconnect from instance
// @Description Disconnect from instance
// @Tags Instance
// @Accept json
// @Produce json
// @Success 200 {object} gin.H "Instance disconnected successfully"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /instance/disconnect [post]
func (i *instanceHandler) Disconnect(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	updateInstance, err := i.instanceService.Disconnect(instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Set("instance", updateInstance)

	ctx.JSON(http.StatusOK, gin.H{"message": "success"})
}

// Logout from instance
// @Summary Logout from instance
// @Description Logout from instance
// @Tags Instance
// @Accept json
// @Produce json
// @Success 200 {object} gin.H "Instance logged out successfully"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /instance/logout [delete]
func (i *instanceHandler) Logout(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}

	updateInstance, err := i.instanceService.Logout(instance)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.Set("instance", updateInstance)

	ctx.JSON(http.StatusOK, gin.H{"message": "success"})
}

// Get instance status
// @Summary Get instance status
// @Description Get instance status
// @Tags Instance
// @Accept json
// @Produce json
// @Success 200 {object} gin.H "Instance status"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /instance/status [get]
func (i *instanceHandler) Status(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}

	status, err := i.instanceService.Status(instance)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": status})
}

// Get instance QR code
// @Summary Get instance QR code
// @Description Get instance QR code
// @Tags Instance
// @Accept json
// @Produce json
// @Success 200 {object} gin.H "Instance QR code"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /instance/qr [get]
func (i *instanceHandler) Qr(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "instance not found"})
		return
	}

	qrcode, err := i.instanceService.GetQr(instance)
	if err != nil {
		if errors.Is(err, instance_service.ErrSessionAlreadyLoggedIn) {
			ctx.JSON(http.StatusConflict, gin.H{"error": err.Error(), "connected": true})
			return
		}
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": qrcode})
}

// Request pairing code
// @Summary Request pairing code
// @Description Request pairing code
// @Tags Instance
// @Accept json
// @Produce json
// @Param instance body instance_service.PairStruct true "Instance data"
// @Success 200 {object} gin.H "Pairing code"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /instance/pair [post]
func (i *instanceHandler) Pair(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *instance_service.PairStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Phone == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone is required"})
		return
	}

	pairingCode, err := i.instanceService.Pair(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": pairingCode})
}

// Get all instances
// @Summary Get all instances
// @Description Get all instances
// @Tags Instance
// @Accept json
// @Produce json
// @Success 200 {object} gin.H "All instances"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /instance/all [get]
func (i *instanceHandler) All(ctx *gin.Context) {
	instances, err := i.instanceService.GetAll()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": instances})
}

// Get instance
// @Summary Get instance
// @Description Get instance
// @Tags Instance
// @Accept json
// @Produce json
// @Param instanceId path string true "Instance Id"
// @Success 200 {object} gin.H "Instance"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /instance/info/{instanceId} [get]
func (i *instanceHandler) Info(ctx *gin.Context) {
	instanceId := ctx.Param("instanceId")

	if instanceId == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "instanceId is required"})
		return
	}

	instance, err := i.instanceService.Info(instanceId)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": instance})
}

// Delete instance
// @Summary Delete instance
// @Description Delete instance
// @Tags Instance
// @Accept json
// @Produce json
// @Param instanceId path string true "Instance Id"
// @Success 200 {object} gin.H "Instance deleted successfully"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /instance/delete/{instanceId} [delete]
func (i *instanceHandler) Delete(ctx *gin.Context) {
	instanceId := ctx.Param("instanceId")

	if instanceId == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "instanceId is required"})
		return
	}

	err := i.instanceService.Delete(instanceId)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success"})
}

// Set proxy
// @Summary Set proxy configuration
// @Description Set proxy configuration for an instance
// @Tags Instance
// @Accept json
// @Produce json
// @Param instanceId path string true "Instance id"
// @Param proxy body instance_service.SetProxyStruct true "Proxy configuration"
// @Success 200 {object} gin.H "Proxy set successfully"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /instance/proxy/{instanceId} [post]
func (i *instanceHandler) SetProxy(ctx *gin.Context) {
	instanceId := ctx.Param("instanceId")

	if instanceId == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "instanceId is required"})
		return
	}

	var data *instance_service.SetProxyStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate required fields
	if data.Host == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "host is required"})
		return
	}

	if data.Port == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "port is required"})
		return
	}

	err = i.instanceService.SetProxyFromStruct(instanceId, data)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	responseData := gin.H{
		"protocol": utils.NormalizeProxyProtocol(data.Protocol, data.Port),
		"host":     data.Host,
		"port":     data.Port,
		"hasAuth":  data.Username != "" && data.Password != "",
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": responseData})
}

// Delete proxy
// @Summary Delete proxy
// @Description Delete proxy
// @Tags Instance
// @Accept json
// @Produce json
// @Param instanceId path string true "Instance id"
// @Success 200 {object} gin.H "Proxy deleted successfully"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /instance/proxy/{instanceId} [delete]
func (i *instanceHandler) DeleteProxy(ctx *gin.Context) {
	instanceId := ctx.Param("instanceId")

	if instanceId == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}

	err := i.instanceService.RemoveProxy(instanceId)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success"})
}

// Force reconnect
// @Summary Force reconnect
// @Description Force reconnect
// @Tags Instance
// @Accept json
// @Produce json
// @Param instanceId path string true "Instance Id"
// @Param instance body instance_service.ForceReconnectStruct true "Instance data"
// @Success 200 {object} gin.H "Instance force reconnected successfully"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /instance/forcereconnect/{instanceId} [post]
func (i *instanceHandler) ForceReconnect(ctx *gin.Context) {
	instanceId := ctx.Param("instanceId")

	if instanceId == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "instanceId is required"})
		return
	}

	var data *instance_service.ForceReconnectStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var number string
	if data.Number == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "number is required"})
		return
	}

	number = data.Number

	err = i.instanceService.ForceReconnect(instanceId, number)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success"})
}

type GetLogsQuery struct {
	StartDate string `form:"start_date"`
	EndDate   string `form:"end_date"`
	Level     string `form:"level"`
	Limit     int    `form:"limit"`
}

// GetLogs returns the log entries for an instance
// @Summary Get instance logs
// @Description Returns log entries for an instance, filterable by date range, level and limit
// @Tags Instance
// @Produce json
// @Param instanceId path string true "Instance Id"
// @Param start_date query string false "Start date (YYYY-MM-DD, defaults to 7 days ago)"
// @Param end_date query string false "End date (YYYY-MM-DD, defaults to now)"
// @Param level query string false "Log level filter"
// @Param limit query int false "Max number of entries"
// @Success 200 {object} gin.H "Logs"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /instance/logs/{instanceId} [get]
func (h *instanceHandler) GetLogs(c *gin.Context) {
	instanceId := c.Param("instanceId")

	var query GetLogsQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Converte as datas
	startDate, err := time.Parse("2006-01-02", query.StartDate)
	if err != nil {
		startDate = time.Now().AddDate(0, 0, -7) // Default: 7 dias atrás
	}

	endDate, err := time.Parse("2006-01-02", query.EndDate)
	if err != nil {
		endDate = time.Now()
	}

	// Ajusta o endDate para o final do dia
	endDate = time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 23, 59, 59, 999999999, time.UTC)

	if query.Limit == 0 {
		query.Limit = 100 // Default: 100 registros
	}

	logs, err := h.instanceService.GetLogs(instanceId, startDate, endDate, query.Level, query.Limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, logs)
}

// GetAdvancedSettings retrieves advanced settings for an instance
// @Summary Get advanced settings
// @Description Get advanced settings for a specific instance
// @Tags Instance
// @Produce json
// @Param instanceId path string true "Instance ID"
// @Success 200 {object} instance_model.AdvancedSettings "Advanced settings retrieved successfully"
// @Failure 400 {object} gin.H "Invalid instance ID"
// @Failure 404 {object} gin.H "Instance not found"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /instance/{instanceId}/advanced-settings [get]
func (h *instanceHandler) GetAdvancedSettings(c *gin.Context) {
	instanceId := c.Param("instanceId")

	if instanceId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "instanceId is required"})
		return
	}

	settings, err := h.instanceService.GetAdvancedSettings(instanceId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, settings)
}

// UpdateAdvancedSettings updates advanced settings for an instance
// @Summary Update advanced settings
// @Description Update advanced settings for a specific instance
// @Tags Instance
// @Accept json
// @Produce json
// @Param instanceId path string true "Instance ID"
// @Param settings body instance_model.AdvancedSettings true "Advanced settings data"
// @Success 200 {object} gin.H "Advanced settings updated successfully"
// @Failure 400 {object} gin.H "Invalid request data"
// @Failure 404 {object} gin.H "Instance not found"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /instance/{instanceId}/advanced-settings [put]
func (h *instanceHandler) UpdateAdvancedSettings(c *gin.Context) {
	instanceId := c.Param("instanceId")

	if instanceId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "instanceId is required"})
		return
	}

	var settings instance_model.AdvancedSettings
	if err := c.ShouldBindJSON(&settings); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := h.instanceService.UpdateAdvancedSettings(instanceId, &settings)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "Advanced settings updated successfully",
		"settings": settings,
	})
}

func NewInstanceHandler(instanceService instance_service.InstanceService, config *config.Config) InstanceHandler {
	return &instanceHandler{instanceService: instanceService, config: config}
}
