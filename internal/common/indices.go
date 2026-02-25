package common

import (
	"fmt"
	"log"
	"time"
)

const solution = "siem"

var loc = time.UTC

// InitTimezone sets the global timezone used by Now().
func InitTimezone(tz string) {
	l, err := time.LoadLocation(tz)
	if err != nil {
		log.Printf("[common] 타임존 로드 실패(%s), KST 사용: %v", tz, err)
		l = time.FixedZone("KST", 9*60*60)
	}
	loc = l
	log.Printf("[common] 타임존: %s", loc)
}

// Now returns current time in the configured timezone.
func Now() time.Time { return time.Now().In(loc) }

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
