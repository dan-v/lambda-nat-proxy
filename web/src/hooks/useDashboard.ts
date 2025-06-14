import { useState, useEffect, useCallback, useRef } from 'react';
import { DashboardData } from '../types';

export const useDashboard = (useWebSocket = true) => {
  const [data, setData] = useState<DashboardData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [connected, setConnected] = useState(false);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimeoutRef = useRef<number | null>(null);

  const fetchData = useCallback(async () => {
    try {
      const response = await fetch('/api/dashboard');
      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
      }
      const dashboardData = await response.json();
      setData(dashboardData);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error');
      console.error('Failed to fetch dashboard data:', err);
    } finally {
      setLoading(false);
    }
  }, []);

  const connectWebSocket = useCallback(() => {
    if (!useWebSocket) return;

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws`;
    
    try {
      const ws = new WebSocket(wsUrl);
      wsRef.current = ws;

      ws.onopen = () => {
        console.log('WebSocket connected');
        setConnected(true);
        setError(null);
        setLoading(false);
        
        // Clear any pending reconnect timeout
        if (reconnectTimeoutRef.current) {
          window.clearTimeout(reconnectTimeoutRef.current);
          reconnectTimeoutRef.current = null;
        }
      };

      ws.onmessage = (event) => {
        try {
          const dashboardData = JSON.parse(event.data);
          setData(dashboardData);
          setError(null);
        } catch (err) {
          console.error('Failed to parse WebSocket message:', err);
        }
      };

      ws.onclose = () => {
        console.log('WebSocket disconnected');
        setConnected(false);
        wsRef.current = null;
        
        // Attempt to reconnect after 3 seconds
        reconnectTimeoutRef.current = window.setTimeout(() => {
          console.log('Attempting to reconnect WebSocket...');
          connectWebSocket();
        }, 3000);
      };

      ws.onerror = (error) => {
        console.error('WebSocket error:', error);
        setError('WebSocket connection failed');
      };

    } catch (err) {
      console.error('Failed to create WebSocket connection:', err);
      setError('Failed to establish real-time connection');
      // Fallback to HTTP polling
      const interval = setInterval(fetchData, 2000);
      return () => clearInterval(interval);
    }
  }, [useWebSocket, fetchData]);

  useEffect(() => {
    if (useWebSocket) {
      connectWebSocket();
    } else {
      // Fallback to HTTP polling
      fetchData();
      const interval = setInterval(fetchData, 2000);
      return () => clearInterval(interval);
    }

    return () => {
      if (wsRef.current) {
        wsRef.current.close();
      }
      if (reconnectTimeoutRef.current) {
        window.clearTimeout(reconnectTimeoutRef.current);
      }
    };
  }, [useWebSocket, connectWebSocket, fetchData]);

  return { 
    data, 
    loading, 
    error, 
    connected, 
    refetch: fetchData 
  };
};