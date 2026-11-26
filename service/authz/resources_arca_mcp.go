package authz

var (
	ArcaFeedbackRead      = Permission{Resource: "arca_feedback", Action: ActionRead}
	ArcaFeedbackWrite     = Permission{Resource: "arca_feedback", Action: ActionWrite}
	ArcaMonitoringRead    = Permission{Resource: "arca_monitoring", Action: ActionRead}
	ArcaEntitlementsRead  = Permission{Resource: "arca_entitlements", Action: ActionRead}
	ArcaEntitlementsWrite = Permission{Resource: "arca_entitlements", Action: ActionWrite}
)

func init() {
	RegisterResource(ResourceDefinition{
		Resource: ArcaFeedbackRead.Resource,
		LabelKey: "Arca Feedback",
		Actions: []ActionDefinition{
			{Action: ActionRead, LabelKey: "View", DescriptionKey: "List and read feedback in Arca MCP.", DefaultRoles: []string{BuiltInRoleAdmin}},
			{Action: ActionWrite, LabelKey: "Edit", DescriptionKey: "Create, reply to, and update the status of feedback in Arca MCP."},
		},
	})
	RegisterResource(ResourceDefinition{
		Resource: ArcaMonitoringRead.Resource,
		LabelKey: "Arca Request Diagnostics",
		Actions: []ActionDefinition{
			{Action: ActionRead, LabelKey: "View", DescriptionKey: "Read redacted request diagnostics in Arca MCP."},
		},
	})
	RegisterResource(ResourceDefinition{
		Resource: ArcaEntitlementsRead.Resource,
		LabelKey: "Arca Entitlements",
		Actions: []ActionDefinition{
			{Action: ActionRead, LabelKey: "View", DescriptionKey: "Preview model access and usage limits in Arca MCP."},
			{Action: ActionWrite, LabelKey: "Edit", DescriptionKey: "Assign or clear Arca subscriptions after confirmation."},
		},
	})
}
