package sippyserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/jackc/pgtype"
	"github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/db/models"
	log "github.com/sirupsen/logrus"
)

const (
	// MaxConversationSizeBytes is the maximum size of a conversation's messages in bytes (4MB)
	MaxConversationSizeBytes = 4194304
)

// CreateChatConversationRequest is the request payload for creating a new chat conversation
type CreateChatConversationRequest struct {
	Messages []map[string]interface{} `json:"messages"`
	Metadata map[string]interface{}   `json:"metadata,omitempty"`
	ParentID *uuid.UUID               `json:"parent_id,omitempty"`
}

// ChatConversationResponse is the response for a chat conversation with HATEOAS links
type ChatConversationResponse struct {
	ID        uuid.UUID         `json:"id"`
	CreatedAt string            `json:"created_at"`
	User      string            `json:"user"`
	Links     map[string]string `json:"links"`
}

// jsonCreateChatConversation handles POST requests to save a new chat conversation
func (s *Server) jsonCreateChatConversation(w http.ResponseWriter, req *http.Request) {
	user := getUserForRequest(req)
	if user == "" {
		failureResponse(w, http.StatusUnauthorized, "User authentication required")
		return
	}

	var request CreateChatConversationRequest
	if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
		log.WithError(err).Error("error parsing chat conversation request")
		failureResponse(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	// Validate messages
	if len(request.Messages) == 0 {
		failureResponse(w, http.StatusBadRequest, "Messages are required")
		return
	}

	// Marshal messages to JSON
	messagesJSON, err := json.Marshal(request.Messages)
	if err != nil {
		log.WithError(err).Error("error marshaling messages")
		failureResponse(w, http.StatusInternalServerError, "Failed to process messages")
		return
	}

	// Reject payloads larger than MaxConversationSizeBytes to prevent abuse
	if len(messagesJSON) > MaxConversationSizeBytes {
		failureResponse(w, http.StatusBadRequest, fmt.Sprintf("Conversation too large (maximum %d bytes)", MaxConversationSizeBytes))
		return
	}

	// Marshal metadata to JSON if provided
	var metadataJSONB pgtype.JSONB
	if request.Metadata != nil {
		metadataJSON, err := json.Marshal(request.Metadata)
		if err != nil {
			log.WithError(err).Error("error marshaling metadata")
			failureResponse(w, http.StatusInternalServerError, "Failed to process metadata")
			return
		}
		if err := metadataJSONB.Set(metadataJSON); err != nil {
			log.WithError(err).Error("error setting metadata JSONB")
			failureResponse(w, http.StatusInternalServerError, "Failed to process metadata")
			return
		}
	}

	// Create the conversation
	conversation := models.ChatConversation{
		User:     user,
		ParentID: request.ParentID,
	}
	if err := conversation.Messages.Set(messagesJSON); err != nil {
		log.WithError(err).Error("error setting messages JSONB")
		failureResponse(w, http.StatusInternalServerError, "Failed to process messages")
		return
	}
	if request.Metadata != nil {
		conversation.Metadata = metadataJSONB
	}

	if err := s.db.DB.Create(&conversation).Error; err != nil {
		log.WithError(err).Error("error creating chat conversation")
		failureResponse(w, http.StatusInternalServerError, "Failed to save conversation")
		return
	}

	baseURL := api.GetBaseURL(req)
	response := ChatConversationResponse{
		ID:        conversation.ID,
		CreatedAt: conversation.CreatedAt.Format(time.RFC3339),
		User:      conversation.User,
		Links: map[string]string{
			"self": fmt.Sprintf("%s/api/chat/conversations/%s", baseURL, conversation.ID.String()),
		},
	}

	log.WithFields(log.Fields{
		"user":           user,
		"conversationID": conversation.ID,
	}).Info("chat conversation created")

	api.RespondWithJSON(http.StatusCreated, w, response)
}

// jsonGetChatConversation handles GET requests to retrieve a chat conversation by ID
func (s *Server) jsonGetChatConversation(w http.ResponseWriter, req *http.Request) {
	vars := mux.Vars(req)
	idStr := vars["id"]

	conversationID, err := uuid.Parse(idStr)
	if err != nil {
		failureResponse(w, http.StatusBadRequest, "Invalid conversation ID format")
		return
	}

	var conversation models.ChatConversation
	if err := s.db.DB.First(&conversation, "id = ?", conversationID).Error; err != nil {
		log.WithError(err).Warn("conversation not found")
		failureResponse(w, http.StatusNotFound, "Conversation not found")
		return
	}

	// Add HATEOAS links
	baseURL := api.GetBaseURL(req)
	conversation.Links = map[string]string{
		"self": fmt.Sprintf("%s/api/chat/conversations/%s", baseURL, conversation.ID.String()),
	}
	if conversation.ParentID != nil {
		conversation.Links["parent"] = fmt.Sprintf("%s/api/chat/conversations/%s", baseURL, conversation.ParentID.String())
	}

	api.RespondWithJSON(http.StatusOK, w, conversation)
}
