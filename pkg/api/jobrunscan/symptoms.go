package jobrunscan

import (
	"errors"
	"fmt"
	"net/http"

	sippyapi "github.com/openshift/sippy/pkg/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models/jobrunscan"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// validateSymptom ensures the Symptom record coming into the API appears valid.
func validateSymptom(dbc *gorm.DB, symptom jobrunscan.Symptom) error {
	if !validIdentifierRegex.MatchString(symptom.ID) {
		return fmt.Errorf("invalid id for a symptom: %s", symptom.ID)
	}

	if symptom.Summary == "" {
		return fmt.Errorf("summary is required for a symptom")
	}

	if symptom.MatcherType == "" {
		return fmt.Errorf("matcher_type is required for a symptom")
	}

	// Validate matcher_type enum
	validMatcherTypes := map[string]bool{
		jobrunscan.MatcherTypeString: true,
		jobrunscan.MatcherTypeRegex:  true,
		jobrunscan.MatcherTypeFile:   true,
		jobrunscan.MatcherTypeCEL:    true,
	}
	if !validMatcherTypes[symptom.MatcherType] {
		return fmt.Errorf("invalid matcher_type: %s (must be one of: string, regex, none, cel)", symptom.MatcherType)
	}

	// Validate required fields based on matcher_type
	if symptom.MatcherType != jobrunscan.MatcherTypeCEL {
		// Non-CEL matchers require file_pattern
		if symptom.FilePattern == "" {
			return fmt.Errorf("file_pattern is required for matcher_type: %s", symptom.MatcherType)
		}
	} else {
		// CEL matchers require match_string
		if symptom.MatchString == "" {
			return fmt.Errorf("match_string is required for matcher_type: cel")
		}
	}

	// Validate that referenced label IDs exist
	if len(symptom.LabelIDs) > 0 {
		var count int64
		res := dbc.Model(&jobrunscan.Label{}).Where("id = ANY(?)", symptom.LabelIDs).Count(&count)
		if res.Error != nil {
			return fmt.Errorf("error validating label_ids: %v", res.Error)
		}
		if int(count) != len(symptom.LabelIDs) {
			return fmt.Errorf("some label_ids do not exist in the database")
		}
	}

	return nil
}

// GetSymptom retrieves a single symptom by ID
func GetSymptom(dbc *db.DB, id string, req *http.Request) (*jobrunscan.Symptom, error) {
	var symptom jobrunscan.Symptom
	res := dbc.DB.First(&symptom, "id = ?", id)
	if res.Error != nil {
		if errors.Is(res.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		log.WithError(res.Error).Errorf("error looking up symptom: %s", id)
		return nil, res.Error
	}
	injectSymptomHATEOASLinks(&symptom, sippyapi.GetBaseURL(req))
	return &symptom, nil
}

// ListSymptoms retrieves all symptoms
func ListSymptoms(dbc *db.DB, req *http.Request) ([]jobrunscan.Symptom, error) {
	var symptoms []jobrunscan.Symptom
	res := dbc.DB.Order("id").Find(&symptoms)
	if res.Error != nil {
		log.WithError(res.Error).Error("error listing symptoms")
		return nil, res.Error
	}
	for i := range symptoms {
		injectSymptomHATEOASLinks(&symptoms[i], sippyapi.GetBaseURL(req))
	}
	return symptoms, nil
}

// CreateSymptom creates a new symptom
func CreateSymptom(dbc *gorm.DB, symptom jobrunscan.Symptom, user string, req *http.Request) (jobrunscan.Symptom, error) {

	// Generate unique ID from summary
	uniqueID, err := generateUniqueIDFromTitle(dbc, symptom.Summary, "job_run_symptoms")
	if err != nil {
		log.WithError(err).Error("error generating unique ID for symptom")
		return symptom, err
	}
	symptom.ID = uniqueID

	err = validateSymptom(dbc, symptom)
	if err != nil {
		log.WithError(err).Error("error validating symptom")
		return symptom, err
	}

	// Set user tracking fields
	symptom.CreatedBy = user
	symptom.UpdatedBy = user

	res := dbc.Create(&symptom)
	if res.Error != nil {
		log.WithError(res.Error).Error("error creating symptom")
		return symptom, res.Error
	}
	log.WithField("symptomID", symptom.ID).Infof("symptom created by user: %s", user)
	injectSymptomHATEOASLinks(&symptom, sippyapi.GetBaseURL(req))
	return symptom, nil
}

// UpdateSymptom updates an existing symptom
func UpdateSymptom(dbc *gorm.DB, symptom jobrunscan.Symptom, user string, req *http.Request) (jobrunscan.Symptom, error) {
	err := validateSymptom(dbc, symptom)
	if err != nil {
		log.WithError(err).Error("error validating symptom")
		return symptom, err
	}

	// Ensure the record exists
	var existingSymptom jobrunscan.Symptom
	res := dbc.First(&existingSymptom, "id = ?", symptom.ID)
	if res.Error != nil {
		log.WithError(res.Error).Errorf("error looking up existing symptom: %s", symptom.ID)
		return symptom, res.Error
	}

	// Preserve created_by and set updated_by
	symptom.CreatedBy = existingSymptom.CreatedBy
	symptom.UpdatedBy = user
	symptom.CreatedAt = existingSymptom.CreatedAt

	// Use Save to update all fields
	res = dbc.Save(&symptom)
	if res.Error != nil {
		log.WithError(res.Error).Error("error updating symptom")
		return symptom, res.Error
	}

	log.WithField("symptomID", symptom.ID).Infof("symptom updated by user: %s", user)
	injectSymptomHATEOASLinks(&symptom, sippyapi.GetBaseURL(req))
	return symptom, nil
}

// DeleteSymptom soft-deletes a symptom
func DeleteSymptom(dbc *gorm.DB, id, user string) error {
	var symptom jobrunscan.Symptom
	res := dbc.First(&symptom, "id = ?", id)
	if res.Error != nil {
		return fmt.Errorf("error finding symptom to delete: %v", res.Error)
	}

	// Update the UpdatedBy field before soft delete
	symptom.UpdatedBy = user
	res = dbc.Save(&symptom)
	if res.Error != nil {
		return fmt.Errorf("error updating symptom before delete: %v", res.Error)
	}

	res = dbc.Delete(&symptom)
	if res.Error != nil {
		return fmt.Errorf("error deleting symptom: %v", res.Error)
	}

	log.WithField("symptomID", id).Infof("symptom deleted by user: %s", user)
	return nil
}

const (
	symptomLink = "%s/api/jobs/symptoms/%s"
)

// injectSymptomHATEOASLinks adds restful links clients can follow for this symptom record.
func injectSymptomHATEOASLinks(symptom *jobrunscan.Symptom, baseURL string) {
	symptom.Links = map[string]string{
		"self": fmt.Sprintf(symptomLink, baseURL, symptom.ID),
	}
}
