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

// validateLabel ensures the Label record coming into the API appears valid.
// update parameter controls whether this is for create (false) or update (true).
func validateLabel(label jobrunscan.Label) error {
	if label.LabelTitle == "" {
		return fmt.Errorf("label_title is required for a label")
	}
	if !ValidIdentifierRegex.MatchString(label.ID) {
		return fmt.Errorf("invalid id for a label: %s", label.ID)
	}

	return nil
}

// GetLabel retrieves a single label by ID
func GetLabel(dbc *db.DB, id string, req *http.Request) (*jobrunscan.Label, error) {
	var label jobrunscan.Label
	res := dbc.DB.First(&label, "id = ?", id)
	if res.Error != nil {
		if errors.Is(res.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		log.WithError(res.Error).Errorf("error looking up label: %s", id)
		return nil, res.Error
	}
	injectLabelHATEOASLinks(&label, sippyapi.GetBaseURL(req))
	return &label, nil
}

// ListLabels retrieves all labels
func ListLabels(dbc *db.DB, req *http.Request) ([]jobrunscan.Label, error) {
	var labels []jobrunscan.Label
	res := dbc.DB.Order("id").Find(&labels)
	if res.Error != nil {
		log.WithError(res.Error).Error("error listing labels")
		return nil, res.Error
	}
	for i := range labels {
		injectLabelHATEOASLinks(&labels[i], sippyapi.GetBaseURL(req))
	}
	return labels, nil
}

// CreateLabel creates a new label
func CreateLabel(dbc *gorm.DB, label jobrunscan.Label, user string, req *http.Request) (jobrunscan.Label, error) {
	// Generate unique ID from label_title
	uniqueID, err := generateUniqueIDFromTitle(dbc, label.LabelTitle, "job_run_labels")
	if err != nil {
		log.WithError(err).Error("error generating unique ID for label")
		return label, err
	}
	label.ID = uniqueID

	err = validateLabel(label)
	if err != nil {
		log.WithError(err).Error("error validating label")
		return label, err
	}

	// Set user tracking fields
	label.CreatedBy = user
	label.UpdatedBy = user

	res := dbc.Create(&label)
	if res.Error != nil {
		log.WithError(res.Error).Error("error creating label")
		return label, res.Error
	}
	log.WithField("labelID", label.ID).Infof("label created by user: %s", user)
	injectLabelHATEOASLinks(&label, sippyapi.GetBaseURL(req))
	return label, nil
}

// UpdateLabel updates an existing label
func UpdateLabel(dbc *gorm.DB, label jobrunscan.Label, user string, req *http.Request) (jobrunscan.Label, error) {
	err := validateLabel(label)
	if err != nil {
		log.WithError(err).Error("error validating label")
		return label, err
	}

	// Ensure the record exists
	var existingLabel jobrunscan.Label
	res := dbc.First(&existingLabel, "id = ?", label.ID)
	if res.Error != nil {
		log.WithError(res.Error).Errorf("error looking up existing label: %s", label.ID)
		return label, res.Error
	}

	// Preserve created_by and set updated_by
	label.CreatedBy = existingLabel.CreatedBy
	label.UpdatedBy = user
	label.CreatedAt = existingLabel.CreatedAt

	// Use Save to update all fields
	res = dbc.Save(&label)
	if res.Error != nil {
		log.WithError(res.Error).Error("error updating label")
		return label, res.Error
	}

	log.WithField("labelID", label.ID).Infof("label updated by user: %s", user)
	injectLabelHATEOASLinks(&label, sippyapi.GetBaseURL(req))
	return label, nil
}

// DeleteLabel soft-deletes a label
func DeleteLabel(dbc *gorm.DB, id, user string) error {
	var label jobrunscan.Label
	res := dbc.First(&label, "id = ?", id)
	if res.Error != nil {
		return fmt.Errorf("error finding label to delete: %v", res.Error)
	}

	// Update the UpdatedBy field before soft delete
	label.UpdatedBy = user
	res = dbc.Save(&label)
	if res.Error != nil {
		return fmt.Errorf("error updating label before delete: %v", res.Error)
	}

	res = dbc.Delete(&label)
	if res.Error != nil {
		return fmt.Errorf("error deleting label: %v", res.Error)
	}

	log.WithField("labelID", id).Infof("label deleted by user: %s", user)
	return nil
}

const (
	labelLink = "%s/api/jobs/labels/%s"
)

// injectLabelHATEOASLinks adds restful links clients can follow for this label record.
func injectLabelHATEOASLinks(label *jobrunscan.Label, baseURL string) {
	label.Links = map[string]string{
		"self": fmt.Sprintf(labelLink, baseURL, label.ID),
	}
}
