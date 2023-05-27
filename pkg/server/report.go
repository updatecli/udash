package server

type PipelineReportSource struct {
	// Name holds the source name
	Name string
	/*
		Result holds the source result, accepted values must be one:
			* "SUCCESS"
			* "FAILURE"
			* "ATTENTION"
			* "SKIPPED"
	*/
	Result string
	// Information stores the information detected by the source execution such as a version
	Information string
	// Description stores the source execution description
	Description string
}

// PipelineReportCondition holds condition execution result
type PipelineReportCondition struct {
	//Name holds the condition name
	Name string
	/*
		Result holds the condition result, accepted values must be one:
			* "SUCCESS"
			* "FAILURE"
			* "ATTENTION"
			* "SKIPPED"
	*/
	Result string
	// Pass stores the information detected by the condition execution.
	Pass bool
	// Description stores the condition execution description.
	Description string
}

// PipelineReportTarget holds target execution result
type PipelineReportTarget struct {
	// Name holds the target name
	Name string
	/*
		Result holds the target result, accepted values must be one:
			* "SUCCESS"
			* "FAILURE"
			* "ATTENTION"
			* "SKIPPED"
	*/
	Result string
	// OldInformation stores the old information detected by the target execution
	OldInformation string
	// NewInformation stores the new information updated by during the target execution
	NewInformation string
	// Description stores the target execution description
	Description string
	// Files holds the list of files modified by a target execution
	Files   []string
	Changed bool
}

type PipelineReport struct {
	Name       string
	Err        string
	Result     string
	Sources    map[string]PipelineReportSource
	Conditions map[string]PipelineReportCondition
	Targets    map[string]PipelineReportTarget
}
