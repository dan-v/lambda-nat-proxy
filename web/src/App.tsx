import React from 'react';
import { useDashboard } from './hooks/useDashboard';
import { Dashboard } from './components/Dashboard';
import './styles/globals.css';
import './styles/dashboard.css';

const App: React.FC = () => {
  const { data, loading, error, connected } = useDashboard();

  if (error) {
    return (
      <div className="error-container">
        <div className="error-content">
          <div className="error-icon">⚡</div>
          <h1>Connection Failed</h1>
          <p>Unable to connect to Lambda NAT Proxy</p>
          <p className="error-detail">{error}</p>
          <button 
            className="retry-button"
            onClick={() => window.location.reload()}
          >
            Retry Connection
          </button>
        </div>
      </div>
    );
  }

  if (loading && !data) {
    return (
      <div className="loading-container">
        <div className="loading-content">
          <div className="loading-logo">
            <div className="logo-bolt">⚡</div>
            <div className="logo-ring"></div>
          </div>
          <h2>Lambda NAT Proxy</h2>
          <p>Initializing secure tunnel...</p>
          <div className="loading-steps">
            <div className="step active">Connecting to proxy</div>
            <div className="step">Establishing QUIC tunnel</div>
            <div className="step">Starting Lambda functions</div>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="app-container">
      <Dashboard 
        data={data || {
          uptime: '0s',
          status: 'disconnected',
          total_connections: 0,
          bytes_per_second: 0,
          avg_latency: 0,
          public_ip: '',
          sessions: [],
          connections: [],
          top_destinations: [],
          destinations: [],
          history: {
            timestamps: [],
            connection_counts: [],
            byte_rates: [],
            latencies: []
          },
          system: {
            goroutines: 0,
            memoryUsageMB: 0,
            uptime: 0
          },
          system_metrics: {
            goroutines: 0,
            memory_mb: 0,
            cpu_percent: 0
          }
        }}
        loading={loading}
        connected={connected}
      />
    </div>
  );
};

export default App;