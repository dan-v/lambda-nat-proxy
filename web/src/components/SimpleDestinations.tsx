import React from 'react';
import { DestinationStats } from '../types';

interface SimpleDestinationsProps {
  destinations: DestinationStats[];
}

export const SimpleDestinations: React.FC<SimpleDestinationsProps> = ({ destinations }) => {
  const formatBytes = (bytes: number): string => {
    if (bytes >= 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
    if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${bytes} B`;
  };

  const formatDomain = (hostname: string): string => {
    // Remove www prefix and extract main domain (e.g., api.github.com -> github.com)
    const cleaned = hostname.replace(/^www\./, '');
    const parts = cleaned.split('.');
    
    // For domains like api.github.com, we want github.com
    // For domains like s3.amazonaws.com, we want amazonaws.com  
    if (parts.length >= 2) {
      return parts.slice(-2).join('.');
    }
    
    return cleaned;
  };

  const getCategoryColor = (hostname: string): string => {
    const domain = hostname.toLowerCase();
    if (domain.includes('google') || domain.includes('youtube')) return '#4285F4';
    if (domain.includes('amazon') || domain.includes('aws')) return '#FF9900';
    if (domain.includes('microsoft') || domain.includes('azure')) return '#00BCF2';
    if (domain.includes('cloudflare') || domain.includes('cdn')) return '#F38020';
    if (domain.includes('github') || domain.includes('gitlab')) return '#6E5494';
    if (domain.includes('api')) return '#00D09C';
    return '#8E8E93';
  };

  // Group destinations by top-level domain
  const groupedDestinations = destinations.reduce((acc, dest) => {
    const domain = formatDomain(dest.hostname);
    
    if (!acc[domain]) {
      acc[domain] = {
        hostname: domain,
        total_bytes: 0,
        bytes_per_second: 0,
        connection_count: 0,
        sparkline: [],
        last_accessed: new Date().toISOString()
      };
    }
    
    acc[domain].total_bytes += dest.total_bytes;
    acc[domain].bytes_per_second += dest.bytes_per_second;
    acc[domain].connection_count += dest.connection_count;
    
    // Update last_accessed to the most recent
    if (new Date(dest.last_accessed) > new Date(acc[domain].last_accessed)) {
      acc[domain].last_accessed = dest.last_accessed;
    }
    
    return acc;
  }, {} as Record<string, DestinationStats>);

  const sortedDestinations = Object.values(groupedDestinations)
    .sort((a, b) => b.total_bytes - a.total_bytes)
    .slice(0, 8);

  const maxBytes = Math.max(...sortedDestinations.map(d => d.total_bytes), 1);

  return (
    <div className="simple-destinations-container">
      <h2 className="section-title">
        <span className="title-icon">🎯</span>
        Top Destinations
      </h2>
      
      <div className="destinations-grid">
        {sortedDestinations.map((dest) => {
          const percentage = (dest.total_bytes / maxBytes) * 100;
          const isActive = dest.bytes_per_second > 0;
          
          return (
            <div key={dest.hostname} className="destination-card">
              <div className="dest-header">
                <div className="dest-info">
                  <div 
                    className="dest-indicator"
                    style={{ backgroundColor: getCategoryColor(dest.hostname) }}
                  />
                  <span className="dest-name">{dest.hostname}</span>
                  {isActive && <div className="live-dot" />}
                </div>
                <div className="dest-stats">
                  <span className="dest-connections">{dest.connection_count}</span>
                  <span className="dest-size">{formatBytes(dest.total_bytes)}</span>
                </div>
              </div>
              
              <div className="dest-bar-bg">
                <div 
                  className="dest-bar"
                  style={{
                    width: `${percentage}%`,
                    backgroundColor: getCategoryColor(dest.hostname),
                    boxShadow: isActive ? `0 0 8px ${getCategoryColor(dest.hostname)}40` : 'none'
                  }}
                />
              </div>
            </div>
          );
        })}
      </div>
      
      {sortedDestinations.length === 0 && (
        <div className="no-destinations">
          <span className="no-data-icon">🌍</span>
          <p>No destinations yet</p>
        </div>
      )}
    </div>
  );
};