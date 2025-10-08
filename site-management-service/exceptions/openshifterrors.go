package exceptions

type (
	OpenShiftPermissionError struct {
		message string
	}
)

func (e OpenShiftPermissionError) Error() string {
	return e.message
}
