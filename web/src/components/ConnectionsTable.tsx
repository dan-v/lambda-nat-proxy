import React, { useState, useMemo } from 'react';
import { TrackedConnection } from '../types';

interface ConnectionsTableProps {
  connections: TrackedConnection[];
}

export const ConnectionsTable: React.FC<ConnectionsTableProps> = ({ connections }) => {
  const [sortBy, setSortBy] = useState<'time' | 'bytes'>('time');

  const sortedConnections = useMemo(() => {
    return [...connections].sort((a, b) => {
      switch (sortBy) {
        case 'time':
          return new Date(b.last_activity).getTime() - new Date(a.last_activity).getTime();
        case 'bytes':
          return (b.bytes_in + b.bytes_out) - (a.bytes_in + a.bytes_out);
        default:
          return 0;
      }
    }).slice(0, 15); // Show top 15
  }, [connections, sortBy]);

  const formatDuration = (startTime: string, lastActivity: string) => {
    const start = new Date(startTime).getTime();
    const last = new Date(lastActivity).getTime();
    const duration = last - start;
    
    if (duration < 60000) return `${Math.round(duration / 1000)}s`;
    if (duration < 3600000) return `${Math.round(duration / 60000)}m`;
    return `${Math.round(duration / 3600000)}h`;
  };

  const getStateColor = (state: string) => {
    switch (state) {
      case 'active': return '#34C759';
      case 'idle': return '#FF9500';
      case 'closing': return '#FF3B30';
      default: return '#8E8E93';
    }
  };

  return (
    <div className="connections-table-container">
      <div className="table-header">
        <h2 className="section-title">
          <span className="title-icon">🔗</span>
          Active Connections
        </h2>
        
        <div className="table-controls">
          <div className="sort-controls">
            <button 
              className={`sort-btn ${sortBy === 'time' ? 'active' : ''}`}
              onClick={() => setSortBy('time')}
            >
              Recent
            </button>
            <button 
              className={`sort-btn ${sortBy === 'bytes' ? 'active' : ''}`}
              onClick={() => setSortBy('bytes')}
            >
              Traffic
            </button>
          </div>
        </div>
      </div>
      
      <div className="connections-list">
        {sortedConnections.map((conn) => (
          <div key={conn.id} className="connection-row">
            <div className="connection-main">
              <div className="connection-endpoints">
                <span className="destination">{conn.destination}</span>
              </div>
              <div className="connection-meta">
                <span className="duration">
                  {formatDuration(conn.start_time, conn.last_activity)}
                </span>
                <span 
                  className="state"
                  style={{ color: getStateColor(conn.state) }}
                >
                  {conn.state}
                </span>
              </div>
            </div>
            
            <div className="connection-stats">
              <div className="stat-item">
                <span className="stat-icon">↓</span>
                <span className="stat-value">{formatBytes(conn.bytes_in)}</span>
              </div>
              <div className="stat-item">
                <span className="stat-icon">↑</span>
                <span className="stat-value">{formatBytes(conn.bytes_out)}</span>
              </div>
            </div>
            
            <div className="connection-graph">
              <div 
                className="traffic-bar in"
                style={{ width: `${Math.min(100, conn.bytes_in / 100000 * 100)}%` }}
              />
              <div 
                className="traffic-bar out"
                style={{ width: `${Math.min(100, conn.bytes_out / 100000 * 100)}%` }}
              />
            </div>
          </div>
        ))}
        
        {sortedConnections.length === 0 && (
          <div className="no-connections">
            <span className="no-data-icon">🔌</span>
            <p>No active connections</p>
          </div>
        )}
      </div>
    </div>
  );
};

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
}