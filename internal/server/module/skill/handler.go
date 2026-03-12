package skill

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Handler handles skill-related HTTP requests
type Handler struct {
	manager *SkillManager
}

// NewHandler creates a new skill handler
func NewHandler(manager *SkillManager) *Handler {
	return &Handler{
		manager: manager,
	}
}

// GetSkillLocations returns all skill locations
func (h *Handler) GetSkillLocations(c *gin.Context) {
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Skill manager not initialized",
		})
		return
	}

	locations := h.manager.ListLocations()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    locations,
	})
}

// AddSkillLocation adds a new skill location
func (h *Handler) AddSkillLocation(c *gin.Context) {
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Skill manager not initialized",
		})
		return
	}

	var req struct {
		Name      string        `json:"name" binding:"required"`
		Path      string        `json:"path" binding:"required"`
		IDESource typ.IDESource `json:"ide_source" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	location, err := h.manager.AddLocation(req.Name, req.Path, req.IDESource)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    location,
		"message": "Skill location added successfully",
	})
}

// RemoveSkillLocation removes a skill location
func (h *Handler) RemoveSkillLocation(c *gin.Context) {
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Skill manager not initialized",
		})
		return
	}

	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Location ID is required",
		})
		return
	}

	if err := h.manager.RemoveLocation(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Skill location removed successfully",
	})
}

// GetSkillLocation retrieves a specific skill location
func (h *Handler) GetSkillLocation(c *gin.Context) {
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Skill manager not initialized",
		})
		return
	}

	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Location ID is required",
		})
		return
	}

	location, err := h.manager.GetLocation(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    location,
	})
}

// RefreshSkillLocation scans a location for updated skill list
func (h *Handler) RefreshSkillLocation(c *gin.Context) {
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Skill manager not initialized",
		})
		return
	}

	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Location ID is required",
		})
		return
	}

	result, err := h.manager.ScanLocation(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
		"message": "Skill location refreshed successfully",
	})
}

// DiscoverIdes scans the home directory for installed IDEs with skills
func (h *Handler) DiscoverIdes(c *gin.Context) {
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Skill manager not initialized",
		})
		return
	}

	result, err := h.manager.DiscoverIdes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// ImportSkillLocations imports discovered skill locations
func (h *Handler) ImportSkillLocations(c *gin.Context) {
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Skill manager not initialized",
		})
		return
	}

	var req struct {
		Locations []typ.SkillLocation `json:"locations" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	imported := []typ.SkillLocation{}
	existingLocations := h.manager.ListLocations()
	existingPaths := make(map[string]bool)
	for _, loc := range existingLocations {
		existingPaths[loc.Path] = true
	}

	for _, loc := range req.Locations {
		// Skip if path already exists
		if existingPaths[loc.Path] {
			continue
		}

		added, err := h.manager.AddLocation(loc.Name, loc.Path, loc.IDESource)
		if err != nil {
			// Log but continue with other locations
			continue
		}
		imported = append(imported, *added)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    imported,
		"message": "Imported " + strconv.Itoa(len(imported)) + " skill locations",
	})
}

// GetSkillContent returns the content of a skill file
func (h *Handler) GetSkillContent(c *gin.Context) {
	locationID := c.Query("location_id")
	skillID := c.Query("skill_id")
	skillPath := c.Query("skill_path")

	if locationID == "" || (skillID == "" && skillPath == "") {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "location_id and either skill_id or skill_path are required",
		})
		return
	}

	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Skill manager not initialized",
		})
		return
	}

	skill, err := h.manager.GetSkillContent(locationID, skillID, skillPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    skill,
	})
}

// ScanIdes scans all IDE locations and returns discovered skills
// This is a comprehensive scan that checks all default IDE locations
func (h *Handler) ScanIdes(c *gin.Context) {
	if h.manager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Skill manager not initialized",
		})
		return
	}

	result, err := h.manager.ScanIdes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}
