import React from 'react';
import { DashboardData } from '../types';
import { SimpleHeader } from './SimpleHeader';
import { LambdaFleet } from './LambdaFleet';
import { SimpleChart } from './SimpleChart';
import { SimpleDestinations } from './SimpleDestinations';
import { ConnectionsTable } from './ConnectionsTable';

interface DashboardProps {
  data: DashboardData;
  loading: boolean;
  connected: boolean;
}

export const Dashboard: React.FC<DashboardProps> = ({ data, connected }) => {
  return (
    <div className="dashboard-container">
      {/* Simple Header */}
      <SimpleHeader data={data} connected={connected} />
      
      {/* Main Grid Layout */}
      <div className="dashboard-grid">
        {/* Lambda Fleet Status */}
        <div className="dashboard-section lambda-fleet">
          <LambdaFleet sessions={data.sessions} />
        </div>
        
        {/* Performance Graph */}
        <div className="dashboard-section performance-graph">
          <SimpleChart data={data} />
        </div>
        
        {/* Destination Analytics */}
        <div className="dashboard-section destination-map">
          <SimpleDestinations destinations={data.top_destinations} />
        </div>
        
        {/* Active Connections */}
        <div className="dashboard-section connections-table">
          <ConnectionsTable connections={data.connections} />
        </div>
      </div>
    </div>
  );
};