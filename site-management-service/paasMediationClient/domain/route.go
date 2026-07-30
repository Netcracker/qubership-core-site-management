package domain

import (
	"reflect"
	"strconv"
	"strings"
)

const (
	RouteKindOpenShift = "Route"
	RouteKindHTTP      = "HTTPRoute"
	RouteKindGRPC      = "GRPCRoute"
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

func routeKindOrDefault(kind string) string {
	if kind == "" {
		return RouteKindOpenShift
	}
	return kind
}

// RouteCacheKey returns a unique cache key for a route within a namespace.
// HTTPRoute and GRPCRoute may share the same metadata.name in Kubernetes.
func RouteCacheKey(route Route) string {
	return routeKindOrDefault(route.Metadata.Kind) + "/" + route.Metadata.Name
}

// NormalizeRouteKind assigns the default OpenShift route kind when kind is absent.
func NormalizeRouteKind(route *Route) {
	if route.Metadata.Kind == "" {
		route.Metadata.Kind = RouteKindOpenShift
	}
}

// IndexRoutesByCacheKey indexes routes by kind/name cache key.
func IndexRoutesByCacheKey(routes []Route) map[string]Route {
	indexed := make(map[string]Route, len(routes))
	for i := range routes {
		NormalizeRouteKind(&routes[i])
		indexed[RouteCacheKey(routes[i])] = routes[i]
	}
	return indexed
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
