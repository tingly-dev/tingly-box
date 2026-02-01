package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ============================================
// Skill Management API Handlers
// ============================================

// GetSkillLocations returns all skill locations
func (s *Server) GetSkillLocations(c *gin.Context) {
	if s.skillManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Skill manager not initialized",
		})
		return
	}

	locations := s.skillManager.ListLocations()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    locations,
	})
}

// AddSkillLocation adds a new skill location
func (s *Server) AddSkillLocation(c *gin.Context) {
	if s.skillManager == nil {
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

	location, err := s.skillManager.AddLocation(req.Name, req.Path, req.IDESource)
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
func (s *Server) RemoveSkillLocation(c *gin.Context) {
	if s.skillManager == nil {
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

	if err := s.skillManager.RemoveLocation(id); err != nil {
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
func (s *Server) GetSkillLocation(c *gin.Context) {
	if s.skillManager == nil {
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

	location, err := s.skillManager.GetLocation(id)
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
func (s *Server) RefreshSkillLocation(c *gin.Context) {
	if s.skillManager == nil {
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

	result, err := s.skillManager.ScanLocation(id)
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
func (s *Server) DiscoverIdes(c *gin.Context) {
	if s.skillManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Skill manager not initialized",
		})
		return
	}

	result, err := s.skillManager.DiscoverIdes()
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
func (s *Server) ImportSkillLocations(c *gin.Context) {
	if s.skillManager == nil {
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
	existingLocations := s.skillManager.ListLocations()
	existingPaths := make(map[string]bool)
	for _, loc := range existingLocations {
		existingPaths[loc.Path] = true
	}

	for _, loc := range req.Locations {
		// Skip if path already exists
		if existingPaths[loc.Path] {
			continue
		}

		added, err := s.skillManager.AddLocation(loc.Name, loc.Path, loc.IDESource)
		if err != nil {
			// Log but continue with other locations
			continue
		}
		imported = append(imported, *added)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    imported,
		"message": "Imported " + string(rune(len(imported))) + " skill locations",
	})
}

// GetSkillContent returns the content of a skill file
func (s *Server) GetSkillContent(c *gin.Context) {
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

	if s.skillManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Skill manager not initialized",
		})
		return
	}

	skill, err := s.skillManager.GetSkillContent(locationID, skillID, skillPath)
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
func (s *Server) ScanIdes(c *gin.Context) {
	if s.skillManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Skill manager not initialized",
		})
		return
	}

	result, err := s.skillManager.ScanIdes()
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
