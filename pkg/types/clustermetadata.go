package types

import (
	"github.com/metalkube/kni-installer/pkg/types/aws"
	"github.com/metalkube/kni-installer/pkg/types/baremetal"
	"github.com/metalkube/kni-installer/pkg/types/libvirt"
	"github.com/metalkube/kni-installer/pkg/types/openstack"
)

// ClusterMetadata contains information
// regarding the cluster that was created by installer.
type ClusterMetadata struct {
	ClusterName             string `json:"clusterName"`
	ClusterID               string `json:"clusterID"`
	ClusterPlatformMetadata `json:",inline"`
}

// ClusterPlatformMetadata contains metadata for platfrom.
type ClusterPlatformMetadata struct {
	AWS       *aws.Metadata       `json:"aws,omitempty"`
	OpenStack *openstack.Metadata `json:"openstack,omitempty"`
	Libvirt   *libvirt.Metadata   `json:"libvirt,omitempty"`
	BareMetal *baremetal.Metadata `json:"baremetal,omitempty"`
}

// Platform returns a string representation of the platform
// (e.g. "aws" if AWS is non-nil).  It returns an empty string if no
// platform is configured.
func (cpm *ClusterPlatformMetadata) Platform() string {
	if cpm == nil {
		return ""
	}
	if cpm.AWS != nil {
		return "aws"
	}
	if cpm.Libvirt != nil {
		return "libvirt"
	}
	if cpm.OpenStack != nil {
		return "openstack"
	}
	if cpm.BareMetal != nil {
		return "baremetal"
	}
	return ""
}
