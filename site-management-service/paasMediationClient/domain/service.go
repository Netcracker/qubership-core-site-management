package domain

type Service struct {
	Metadata Metadata    `json:"metadata"`
	Spec     ServiceSpec `json:"spec"`
}

type ServiceSpec struct {
	Ports     []Port            `json:"ports"`
	Selector  map[string]string `json:"selector"`
	ClusterIP string            `json:"clusterIP"`
	Type      string            `json:"type"`
}

type Port struct {
	Name       string `json:"name"`
	Protocol   string `json:"protocol"`
	Port       int32  `json:"port"`
	TargetPort int32  `json:"targetPort"`
	NodePort   int32  `json:"nodePort,omitempty"`
}

func (s Service) GetId() string {
	if value, ok := s.Metadata.Annotations["netcracker.cloud/tenant.service.id"]; ok {
		return value
	} else {
		return ""
	}
}

func (s Service) GetShowName() string {
	if value, ok := s.Metadata.Annotations["netcracker.cloud/tenant.service.show.name"]; ok {
		return value
	} else {
		return ""
	}
}

func (s Service) GetDescription() string {
	if value, ok := s.Metadata.Annotations["netcracker.cloud/tenant.service.show.description"]; ok {
		return value
	} else {
		return ""
	}
}

func (s Service) GetSuffix() string {
	if value, ok := s.Metadata.Annotations["netcracker.cloud/tenant.service.url.suffix"]; ok {
		return value
	} else {
		return ""
	}
}

func (s Service) GetPrefix() string {
	if value, ok := s.Metadata.Annotations["netcracker.cloud/tenant.service.alias.prefix"]; ok {
		return value
	} else {
		return ""
	}
}
