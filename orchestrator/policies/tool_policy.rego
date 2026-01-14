package tool_policy

default decision = "allow"

# Example: Block dangerous tools
decision = "block" {
	input.tool_name == "dangerous.command"
}

# Example: Require approval for high value transfer
decision = "require_approval" {
	input.tool_name == "payments.transfer"
	input.args.amount > 100
}
