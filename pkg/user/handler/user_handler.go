package user_handler

import (
	"net/http"

	instance_model "github.com/evolution-foundation/evolution-go/pkg/instance/model"
	user_service "github.com/evolution-foundation/evolution-go/pkg/user/service"
	"github.com/gin-gonic/gin"
)

type UserHandler interface {
	GetUser(ctx *gin.Context)
	CheckUser(ctx *gin.Context)
	GetAvatar(ctx *gin.Context)
	GetContacts(ctx *gin.Context)
	GetPrivacy(ctx *gin.Context)
	SetPrivacy(ctx *gin.Context)
	BlockContact(ctx *gin.Context)
	UnblockContact(ctx *gin.Context)
	GetBlockList(ctx *gin.Context)
	SetProfilePicture(ctx *gin.Context)
	SetProfileName(ctx *gin.Context)
	SetProfileStatus(ctx *gin.Context)
	ResolveLid(ctx *gin.Context)
}

type userHandler struct {
	userService user_service.UserService
}

// Get a user
// @Summary Get a user
// @Description Get a user
// @Tags User
// @Accept json
// @Produce json
// @Param message body user_service.CheckUserStruct true "User data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /user/info [post]
func (u *userHandler) GetUser(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *user_service.CheckUserStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(data.Number) < 1 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	uc, err := u.userService.GetUser(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": uc})
}

// Check a user
// @Summary Check a user
// @Description Check a user
// @Tags User
// @Accept json
// @Produce json
// @Param message body user_service.CheckUserStruct true "User data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /user/check [post]
func (u *userHandler) CheckUser(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *user_service.CheckUserStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(data.Number) < 1 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	uc, err := u.userService.CheckUser(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": uc})
}

// Get a user's avatar
// @Summary Get a user's avatar
// @Description Get a user's avatar
// @Tags User
// @Accept json
// @Produce json
// @Param message body user_service.GetAvatarStruct true "Avatar data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /user/avatar [post]
func (u *userHandler) GetAvatar(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *user_service.GetAvatarStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(data.Number) < 1 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	if data.Number == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	pic, err := u.userService.GetAvatar(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": pic})
}

// Get a user's contacts
// @Summary Get a user's contacts
// @Description Get a user's contacts
// @Tags User
// @Accept json
// @Produce json
// @Success 200 {object} gin.H "success"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /user/contacts [get]
func (u *userHandler) GetContacts(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	contacts, err := u.userService.GetContacts(instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": contacts})
}

// Get a user's privacy settings
// @Summary Get a user's privacy settings
// @Description Get a user's privacy settings
// @Tags User
// @Accept json
// @Produce json
// @Success 200 {object} gin.H "success"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /user/privacy [get]
func (u *userHandler) GetPrivacy(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	privacy, err := u.userService.GetPrivacy(instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": privacy})
}

// Set a user's privacy settings
// @Summary Set a user's privacy settings
// @Description Set a user's privacy settings
// @Tags User
// @Accept json
// @Produce json
// @Param message body user_service.PrivacyStruct true "Privacy data"
// @Success 200 {object} gin.H "success"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /user/privacy [post]
func (u *userHandler) SetPrivacy(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *user_service.PrivacyStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.CallAdd == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "call add is required"})
		return
	}

	if data.GroupAdd == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "group add is required"})
		return
	}

	if data.LastSeen == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "last seen is required"})
		return
	}

	if data.Online == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "online is required"})
		return
	}

	if data.Profile == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "profile is required"})
		return
	}

	if data.ReadReceipts == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "read receipts is required"})
		return
	}

	if data.Status == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "status is required"})
		return
	}

	privacy, err := u.userService.SetPrivacy(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": privacy})
}

// Block a contact
// @Summary Block a contact
// @Description Block a contact
// @Tags User
// @Accept json
// @Produce json
// @Param message body user_service.BlockStruct true "Block data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /user/block [post]
func (u *userHandler) BlockContact(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *user_service.BlockStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(data.Number) < 1 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	if data.Number == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	resp, err := u.userService.BlockContact(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": resp})
}

// Unblock a contact
// @Summary Unblock a contact
// @Description Unblock a contact
// @Tags User
// @Accept json
// @Produce json
// @Param message body user_service.BlockStruct true "Block data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /user/unblock [post]
func (u *userHandler) UnblockContact(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *user_service.BlockStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(data.Number) < 1 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	if data.Number == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "phone number is required"})
		return
	}

	resp, err := u.userService.UnlockContact(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": resp})
}

// Get a user's block list
// @Summary Get a user's block list
// @Description Get a user's block list
// @Tags User
// @Accept json
// @Produce json
// @Success 200 {object} gin.H "success"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /user/blocklist [get]
func (u *userHandler) GetBlockList(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	resp, err := u.userService.GetBlockList(instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": resp})
}

// Set a user's profile picture
// @Summary Set a user's profile picture
// @Description Set a user's profile picture
// @Tags User
// @Accept json
// @Produce json
// @Param message body user_service.SetProfilePictureStruct true "Profile picture data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /user/profilePicture [post]
func (u *userHandler) SetProfilePicture(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *user_service.SetProfilePictureStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Image == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "image is required"})
		return
	}

	resp, err := u.userService.SetProfilePicture(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !resp {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set profile picture"})
		return
	}

	responseData := gin.H{"image": data.Image}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": responseData})
}

// Set a user's profile name
// @Summary Set a user's profile name
// @Description Set a user's profile name
// @Tags User
// @Accept json
// @Produce json
// @Param message body user_service.SetProfilePictureStruct true "Profile name data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /user/profileName [post]
func (u *userHandler) SetProfileName(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *user_service.SetProfileNameStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Name == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	resp, err := u.userService.SetProfileName(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !resp {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set profile picture"})
		return
	}

	responseData := gin.H{"name": data.Name}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": responseData})
}

// Set a user's profile status
// @Summary Set a user's profile status
// @Description Set a user's profile status
// @Tags User
// @Accept json
// @Produce json
// @Param message body user_service.SetProfilePictureStruct true "Profile status data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /user/profileStatus [post]
func (u *userHandler) SetProfileStatus(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *user_service.SetProfileStatusStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Status == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	resp, err := u.userService.SetProfileStatus(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if !resp {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set profile picture"})
		return
	}

	responseData := gin.H{"status": data.Status}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": responseData})
}

// Resolve the phone number behind a LID
// @Summary Resolve the phone number behind a LID
// @Description Resolve the phone number behind a LID
// @Tags User
// @Accept json
// @Produce json
// @Param message body user_service.ResolveLidStruct true "Lid data"
// @Success 200 {object} gin.H "success"
// @Failure 400 {object} gin.H "Error on validation"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /user/lid [post]
func (u *userHandler) ResolveLid(ctx *gin.Context) {
	getInstance := ctx.MustGet("instance")

	instance, ok := getInstance.(*instance_model.Instance)
	if !ok {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "instance not found"})
		return
	}

	var data *user_service.ResolveLidStruct
	err := ctx.ShouldBindBodyWithJSON(&data)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if data.Lid == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "lid is required"})
		return
	}

	resp, err := u.userService.ResolveLid(data, instance)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "success", "data": resp})
}

func NewUserHandler(
	userService user_service.UserService,
) UserHandler {
	return &userHandler{
		userService: userService,
	}
}
