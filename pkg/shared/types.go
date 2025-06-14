package shared

// CoordinationData represents the coordination information sent from orchestrator to lambda
type CoordinationData struct {
	SessionID        string `json:"session_id"`
	LaptopPublicIP   string `json:"laptop_public_ip"`
	LaptopPublicPort int    `json:"laptop_public_port"`
	Timestamp        int64  `json:"timestamp"`
}

// LambdaResponse represents the response sent from lambda back to orchestrator
type LambdaResponse struct {
	SessionID        string `json:"session_id"`
	LambdaPublicIP   string `json:"lambda_public_ip"`
	LambdaPublicPort int    `json:"lambda_public_port"`
	Status           string `json:"status"`
	Timestamp        int64  `json:"timestamp"`
}