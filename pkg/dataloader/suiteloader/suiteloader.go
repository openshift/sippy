package suiteloader

import (
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	v1config "github.com/openshift/sippy/pkg/apis/config/v1"
	"github.com/openshift/sippy/pkg/db/models"
)

type SuiteLoader struct {
	config *v1config.SippyConfig
	dbc    *gorm.DB
	errors []error
}

func New(config *v1config.SippyConfig, dbc *gorm.DB) *SuiteLoader {
	return &SuiteLoader{
		config: config,
		dbc:    dbc,
	}
}

func (sl *SuiteLoader) Name() string {
	return "suites"
}

func (sl *SuiteLoader) Load() {
	for _, suiteName := range sl.config.Suites {
		s := models.Suite{}
		res := sl.dbc.Where("name = ?", suiteName).First(&s)
		if res.Error != nil {
			if !errors.Is(res.Error, gorm.ErrRecordNotFound) {
				log.WithError(res.Error).Warningf("couldn't save record for %q", suiteName)
				sl.errors = append(sl.errors, res.Error)
			}
			s = models.Suite{
				Name: suiteName,
			}
			err := sl.dbc.Clauses(clause.OnConflict{UpdateAll: true}).Create(&s).Error
			if err != nil {
				err = errors.Wrapf(err, "error loading suite into db: %s", suiteName)
				log.WithError(err)
				sl.errors = append(sl.errors, res.Error)
			}
			log.WithField("suite", suiteName).Info("created new test suite")
		}
	}
}

func (sl *SuiteLoader) Errors() []error {
	return sl.errors
}
