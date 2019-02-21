package destroy

import (
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/openshift-metalkube/kni-installer/pkg/asset/cluster"
	"github.com/openshift-metalkube/kni-installer/pkg/destroy/providers"
)

// New returns a Destroyer based on `metadata.json` in `rootDir`.
func New(logger logrus.FieldLogger, rootDir string) (providers.Destroyer, error) {
	metadata, err := cluster.LoadMetadata(rootDir)
	if err != nil {
		return nil, err
	}

	platform := metadata.Platform()
	if platform == "" {
		return nil, errors.New("no platform configured in metadata")
	}

	creator, ok := providers.Registry[platform]
	if !ok {
		return nil, errors.Errorf("no destroyers registered for %q", platform)
	}
	return creator(logger, metadata)
}
