package domain

import (
	"reflect"
	"strconv"
	"strings"
)

type Route struct {
	Metadata Metadata  `json:"metadata"`
	Spec     RouteSpec `json:"spec"`
}

type RouteSpec struct {
	Host    string    `json:"host"`
	Path    string    `json:"path"`
	Service Target    `json:"to"`
	Port    RoutePort `json:"port"`
}

type Target struct {
	Name string `json:"name"`
}

type RoutePort struct {
	TargetPort int32 `json:"targetPort"`
}

func (r Route) GetPriority() int {
	if value, ok := r.Metadata.Annotations["netcracker.cloud/tenant.service.tenant.id"]; ok && value == "GENERAL" {
		return -1
	} else {
		if value, ok := r.Metadata.Annotations["netcracker.cloud/tenant.service.order"]; ok {
			if result, err := strconv.Atoi(value); err != nil {
				return result
			}
		}
		return 0
	}
}

func (r Route) GetServiceDescription() string {
	if value, ok := r.Metadata.Annotations["netcracker.cloud/tenant.service.show.description"]; ok {
		return value
	} else {
		return ""
	}
}

func (r Route) GetServiceName() string {
	if value, ok := r.Metadata.Annotations["netcracker.cloud/tenant.service.show.name"]; ok {
		return value
	} else {
		return ""
	}
}

func (r Route) GetServiceSuffix() string {
	if value, ok := r.Metadata.Annotations["netcracker.cloud/tenant.service.url.suffix"]; ok {
		return value
	} else {
		return ""
	}
}

func (r Route) GetServiceId(defaultValue string) string {
	if value, ok := r.Metadata.Annotations["netcracker.cloud/tenant.service.id"]; ok {
		return value
	} else {
		return defaultValue
	}
}

func (r Route) GetTenantId() string {
	if value, ok := r.Metadata.Annotations["netcracker.cloud/tenant.service.tenant.id"]; ok {
		return value
	} else {
		return ""
	}
}

func (r Route) String() string {
	return r.FormatString("")
}

func (r Route) FormatString(leftAlignPrefix string) string {
	b := strings.Builder{}
	b.WriteString(leftAlignPrefix)
	b.WriteString("Metadata:")
	b.WriteString("\n")
	b.WriteString(leftAlignPrefix)
	b.WriteString("\tName: ")
	b.WriteString(r.Metadata.Name)
	b.WriteString("\n")
	b.WriteString(leftAlignPrefix)
	b.WriteString("\tAnnotations: ")
	for k, v := range r.Metadata.Annotations {
		b.WriteString("\n")
		b.WriteString(leftAlignPrefix)
		b.WriteString("\t\t")
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(v)
	}
	b.WriteString("\n")
	b.WriteString(leftAlignPrefix)
	b.WriteString("Spec:")
	b.WriteString("\n")
	b.WriteString(leftAlignPrefix)
	b.WriteString("\tHost: ")
	b.WriteString(r.Spec.Host)
	b.WriteString("\n")
	b.WriteString(leftAlignPrefix)
	b.WriteString("\tService: ")
	b.WriteString("\n")
	b.WriteString(leftAlignPrefix)
	b.WriteString("\t\tName: ")
	b.WriteString(r.Spec.Service.Name)

	return b.String()
}

func (r *Route) MergeRoute(route *Route) {
	if !reflect.DeepEqual(r.Spec.Port, route.Spec.Port) {
		r.Spec.Port = route.Spec.Port
	}
}
