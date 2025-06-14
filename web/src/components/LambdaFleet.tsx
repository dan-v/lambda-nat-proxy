import React from 'react';
import { SessionInfo } from '../types';

interface LambdaFleetProps {
  sessions: SessionInfo[];
}

export const LambdaFleet: React.FC<LambdaFleetProps> = ({ sessions }) => {
  return (
    <div className="lambda-fleet-container">
      <h2 className="section-title">
        <span className="title-icon">⚡</span>
        Lambda Fleet Status
      </h2>
      
      {/* Lambda Details */}
      <div className="lambda-details">
        {sessions.map((session, index) => (
          <div key={session.id} className={`lambda-card ${session.role}`}>
            <div className="lambda-header">
              <span className="lambda-id">Lambda #{index + 1}</span>
              <span className={`lambda-role ${session.role}`}>{session.role}</span>
            </div>
            
            <div className="lambda-stats">
              <div className="stat">
                <span className="stat-label">Public IP</span>
                <span className="stat-value">{session.lambda_public_ip || 'Acquiring...'}</span>
              </div>
              <div className="stat">
                <span className="stat-label">Status</span>
                <span className={`stat-value ${session.health > 0.8 ? 'healthy' : 'unhealthy'}`}>
                  {session.health > 0.8 ? 'Healthy' : 'Degraded'}
                </span>
              </div>
            </div>
            
            <div className="lambda-health-bar">
              <div 
                className="health-fill"
                style={{
                  width: `${session.health * 100}%`,
                  backgroundColor: session.health > 0.8 ? '#34C759' : 
                                 session.health > 0.5 ? '#FF9500' : '#FF3B30'
                }}
              />
            </div>
          </div>
        ))}
      </div>
    </div>
  );
};