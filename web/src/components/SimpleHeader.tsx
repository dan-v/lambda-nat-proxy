import React from 'react';
import { DashboardData } from '../types';
import { formatBytes, formatUptime } from '../utils/formatters';

interface SimpleHeaderProps {
  data: DashboardData;
  connected: boolean;
}

export const SimpleHeader: React.FC<SimpleHeaderProps> = ({ data, connected }) => {
  return (
    <div className="simple-header">
      <div className="header-left">
        <h1 className="app-title">⚡ Lambda NAT Proxy</h1>
        <div className={`connection-status ${connected ? 'connected' : 'disconnected'}`}>
          <div className="status-dot" />
          <span>{connected ? 'Connected' : 'Disconnected'}</span>
        </div>
      </div>
      
      <div className="header-stats">
        <div className="stat-item">
          <span className="stat-label">Throughput</span>
          <span className="stat-value">{formatBytes(data?.bytes_per_second || 0)}/s</span>
        </div>
        <div className="stat-item">
          <span className="stat-label">Connections</span>
          <span className="stat-value">{data?.connections?.length || 0}</span>
        </div>
        <div className="stat-item">
          <span className="stat-label">Uptime</span>
          <span className="stat-value">{formatUptime(data?.uptime || '0s')}</span>
        </div>
        {data?.public_ip && (
          <div className="stat-item">
            <span className="stat-label">Public IP</span>
            <span className="stat-value">{data.public_ip}</span>
          </div>
        )}
      </div>
    </div>
  );
};