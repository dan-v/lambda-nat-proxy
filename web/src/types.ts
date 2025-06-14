export interface TrackedConnection {
  id: string;
  client: string;
  destination: string;
  start_time: string;
  bytes_in: number;
  bytes_out: number;
  last_activity: string;
  latency_ms: number;
  state: string;
}

// Alias for compatibility
export type Connection = TrackedConnection & {
  bytes_sent: number;
  bytes_received: number;
  duration: number;
};

export interface SessionInfo {
  id: string;
  role: string;
  health: number;
  duration: number;
  rtt_ms: number;
  ttl: number;
  status: string;
  started_at: string;
  ttl_seconds: number;
  healthy: boolean;
  lambda_public_ip?: string;
}

// Alias for compatibility
export type Session = SessionInfo;

export interface DestinationStats {
  hostname: string;
  connection_count: number;
  total_bytes: number;
  bytes_per_second: number;
  sparkline: number[];
  last_accessed: string;
}

// Extended destination info for bubble chart
export interface Destination {
  domain: string;
  connectionCount: number;
  totalBytes: number;
}

export interface DashboardData {
  uptime: string;
  status: string;
  total_connections: number;
  bytes_per_second: number;
  avg_latency: number;
  public_ip: string;
  sessions: SessionInfo[];
  connections: TrackedConnection[];
  top_destinations: DestinationStats[];
  destinations: Destination[];
  history: {
    timestamps: number[];
    connection_counts: number[];
    byte_rates: number[];
    latencies: number[];
  };
  system: {
    goroutines: number;
    memoryUsageMB: number;
    uptime: number;
  };
  system_metrics: {
    goroutines: number;
    memory_mb: number;
    cpu_percent?: number;
  };
}