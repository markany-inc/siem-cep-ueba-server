package common

import "fmt"

const solution = "siem"

func RulesIndex(prefix string) string {
	return fmt.Sprintf("%s-%s-common-rules", prefix, solution)
}

func SettingsIndex(prefix string) string {
	return fmt.Sprintf("%s-%s-common-settings", prefix, solution)
}

func BaselinesIndex(prefix string) string {
	return fmt.Sprintf("%s-%s-ueba-baselines", prefix, solution)
}

func FieldMetaIndex(prefix string) string {
	return fmt.Sprintf("%s-%s-common-field-meta", prefix, solution)
}

func LogsIndexPattern(prefix string) string {
	return fmt.Sprintf("%s-%s-event-logs-*", prefix, solution)
}

func AlertsIndexPattern(prefix string) string {
	return fmt.Sprintf("%s-%s-cep-alerts-*", prefix, solution)
}

func ScoresIndexPattern(prefix string) string {
	return fmt.Sprintf("%s-%s-ueba-scores-*", prefix, solution)
}

func DailyAlertsIndex(prefix, day string) string {
	return fmt.Sprintf("%s-%s-cep-alerts-%s", prefix, solution, day)
}

func DailyScoresIndex(prefix, day string) string {
	return fmt.Sprintf("%s-%s-ueba-scores-%s", prefix, solution, day)
}

func DailyLogsIndex(prefix, day string) string {
	return fmt.Sprintf("%s-%s-event-logs-%s", prefix, solution, day)
}
