package snapshot

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/jackc/pgtype"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"

	apitype "github.com/openshift/sippy/pkg/apis/api"
	"github.com/openshift/sippy/pkg/db"
	"github.com/openshift/sippy/pkg/db/models"
)

type Snapshotter struct {
	DBC      *db.DB
	Name     string
	SippyURL string
	Release  string
}

func (s *Snapshotter) Create() error {
	log.Info("creating snapshot")

	// Early check to make sure the name is unique:
	var existing models.APISnapshot
	res := s.DBC.DB.Where("name = ?", s.Name).First(&existing)
	if res.Error != nil && !errors.Is(res.Error, gorm.ErrRecordNotFound) {
		return errors.Wrap(res.Error, "error checking if snapshot exists already with name: "+s.Name)
	}
	if existing.ID != 0 {
		return fmt.Errorf("snapshot already exists: %s", s.Name)
	}

	snapshot := models.APISnapshot{
		Name:    s.Name,
		Release: s.Release,
	}

	health, err := s.getHealth()
	if err != nil {
		return err
	}
	snapshot.OverallHealth = health

	payloadHealth, err := s.getPayloadHealth()
	if err != nil {
		return err
	}
	snapshot.PayloadHealth = payloadHealth

	variantHealth, err := s.getVariantHealth()
	if err != nil {
		return err
	}
	snapshot.VariantHealth = variantHealth

	installHealth, err := s.getInstallHealth()
	if err != nil {
		return err
	}
	snapshot.InstallHealth = installHealth

	upgradeHealth, err := s.getUpgradeHealth()
	if err != nil {
		return err
	}
	snapshot.UpgradeHealth = upgradeHealth

	log.Info("storing snapshot in database")
	err = s.DBC.DB.Create(&snapshot).Error
	if err != nil {
		log.WithError(err).Error("error creating snapshot")
		return errors.Wrapf(err, "error creating snapshot")
	}
	log.Info("snapshot created successfully")

	return nil

}

func (s *Snapshotter) getHealth() (pgtype.JSONB, error) {
	relativeAPI := fmt.Sprintf("/api/health?release=%s", s.Release)
	apiURL := s.SippyURL + relativeAPI
	return get(apiURL, &apitype.Health{})
}

func (s *Snapshotter) getPayloadHealth() (pgtype.JSONB, error) {
	relativeAPI := fmt.Sprintf("/api/releases/health?release=%s", s.Release)
	apiURL := s.SippyURL + relativeAPI
	return get(apiURL, &[]apitype.ReleaseHealthReport{})
}

func (s *Snapshotter) getVariantHealth() (pgtype.JSONB, error) {
	relativeAPI := fmt.Sprintf("/api/variants?release=%s", s.Release)
	apiURL := s.SippyURL + relativeAPI
	return get(apiURL, &[]apitype.Variant{})
}

func (s *Snapshotter) getInstallHealth() (pgtype.JSONB, error) {
	relativeAPI := fmt.Sprintf("/api/install?release=%s", s.Release)
	apiURL := s.SippyURL + relativeAPI
	return get(apiURL, &map[string]interface{}{})
}

func (s *Snapshotter) getUpgradeHealth() (pgtype.JSONB, error) {
	relativeAPI := fmt.Sprintf("/api/upgrade?release=%s", s.Release)
	apiURL := s.SippyURL + relativeAPI
	return get(apiURL, &map[string]interface{}{})
}

// nolint:gosec
func get(url string, data interface{}) (pgtype.JSONB, error) {
	logger := log.WithField("api", url)
	logger.Info("GET")
	res, err := http.Get(url)
	if err != nil {
		return pgtype.JSONB{}, err
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return pgtype.JSONB{}, err
	}
	err = json.Unmarshal(body, data)
	if err != nil {
		return pgtype.JSONB{}, err
	}

	jsonb := pgtype.JSONB{}
	if err := jsonb.Set(data); err != nil {
		logger.WithError(err).Error("error setting jsonb value with api output")
		return pgtype.JSONB{}, err
	}

	return jsonb, nil
}
