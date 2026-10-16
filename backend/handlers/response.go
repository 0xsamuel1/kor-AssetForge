package handlers

import (
    "net/http"
    "github.com/gin-gonic/gin"
)

// BindJSON centralizes request decoding and guarantees a consistent client error.
func BindJSON(c *gin.Context, dst any) bool {
    if err := c.ShouldBindJSON(dst); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
        return false
    }
    return true
}

