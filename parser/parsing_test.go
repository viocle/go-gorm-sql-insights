package parser

import (
	"encoding/json"
	"testing"
)

func TestSQLParsing(t *testing.T) {
	// ridiculous SQL string for testing
	sqlString := "SELECT * FROM (SELECT @row := @row +1 AS row_num, case_page_ranking.* FROM (SELECT @row :=0) AS r_counter, (SELECT inside_joiner.* FROM (SELECT `cases`.case_number AS id, IFNULL(case_sort_attribute_items.value_number, 0) AS val_number FROM `cases` JOIN attributes AS at102 ON at102.organization_id = cases.organization_id AND at102.name = 'WEIGHT' AND at102.deleted_at IS NULL LEFT OUTER JOIN attribute_items AS ai102 ON ai102.organization_id = cases.organization_id AND ai102.reference_id = cases.id AND ai102.deleted_at IS NULL AND ai102.attribute_id = at102.id JOIN attributes AS at202 ON at202.organization_id = cases.organization_id AND at202.name = 'WEIGHT' AND at202.deleted_at IS NULL LEFT OUTER JOIN attribute_items AS ai202 ON ai202.organization_id = cases.organization_id AND ai202.reference_id = cases.id AND ai202.deleted_at IS NULL AND ai202.attribute_id = at202.id LEFT OUTER JOIN attribute_items AS case_sort_attribute_items ON case_sort_attribute_items.reference_id = cases.id AND case_sort_attribute_items.attribute_id = 'attr_2002V276F4OEK0AB' AND case_sort_attribute_items.deleted_at IS NULL WHERE (cases.organization_id = 'org_20020RWDDG6AGQF9' AND cases.deleted_at IS NULL) AND (((IFNULL(cases.status,'') = 'ACTIVE') AND (cases.test IS NULL OR cases.test = FALSE) AND (ai102.value_number IS NOT NULL AND ai102.value_number < 10)) OR ((IFNULL(cases.status,'') = 'ACTIVE') AND (cases.test IS NULL OR cases.test = FALSE) AND (ai202.value_number IS NOT NULL AND ai202.value_number > 20))) GROUP BY cases.case_number, IFNULL(case_sort_attribute_items.value_number, 0) ORDER BY IFNULL(case_sort_attribute_items.value_number, 0) DESC, cases.case_number DESC) AS inside_joiner) AS case_page_ranking) AS final_page_ranking WHERE row_num = 1 OR row_num % 10 = 1"

	f, err := ParseSQL(sqlString)
	if err != nil {
		t.Errorf("Error parsing SQL: %v", err)
		return
	}

	j, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		t.Errorf("Error marshalling ParsedFields: %v", err)
		return
	}
	t.Log(string(j))
}
