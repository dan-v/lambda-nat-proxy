import React, { useRef, useEffect, useState } from 'react';
import { DashboardData } from '../types';

interface SimpleChartProps {
  data: DashboardData;
}

interface DataPoint {
  timestamp: number;
  throughput: number;
  connections: number;
}

export const SimpleChart: React.FC<SimpleChartProps> = ({ data }) => {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [dataHistory, setDataHistory] = useState<DataPoint[]>([]);
  const animationFrameRef = useRef<number | undefined>(undefined);

  // Update data history only when data changes
  useEffect(() => {
    if (!data) return;
    
    setDataHistory(prev => {
      const newPoint: DataPoint = {
        timestamp: Date.now(),
        throughput: data.bytes_per_second || 0,
        connections: data.connections?.length || 0
      };

      const updated = [...prev, newPoint];
      return updated.slice(-60); // Keep 1 minute of data
    });
  }, [data?.bytes_per_second, data?.connections?.length]);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;

    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    let isAnimating = true;

    const resizeCanvas = () => {
      const rect = canvas.getBoundingClientRect();
      const dpr = window.devicePixelRatio || 1;
      
      canvas.width = rect.width * dpr;
      canvas.height = rect.height * dpr;
      
      ctx.scale(dpr, dpr);
      canvas.style.width = rect.width + 'px';
      canvas.style.height = rect.height + 'px';
    };

    const draw = () => {
      if (!isAnimating) return;

      const rect = canvas.getBoundingClientRect();
      const width = rect.width;
      const height = rect.height;

      // Clear canvas
      ctx.clearRect(0, 0, width, height);

      // Draw background
      ctx.fillStyle = 'rgba(0, 0, 0, 0.8)';
      ctx.fillRect(0, 0, width, height);

      // Grid
      ctx.strokeStyle = 'rgba(255, 255, 255, 0.05)';
      ctx.lineWidth = 1;
      
      for (let i = 0; i <= 10; i++) {
        const x = (i / 10) * width;
        ctx.beginPath();
        ctx.moveTo(x, 0);
        ctx.lineTo(x, height);
        ctx.stroke();
      }
      
      for (let i = 0; i <= 4; i++) {
        const y = (i / 4) * height;
        ctx.beginPath();
        ctx.moveTo(0, y);
        ctx.lineTo(width, y);
        ctx.stroke();
      }

      if (dataHistory.length < 2) {
        ctx.fillStyle = 'rgba(255, 255, 255, 0.4)';
        ctx.font = '14px -apple-system, BlinkMacSystemFont, sans-serif';
        ctx.textAlign = 'center';
        ctx.fillText('Collecting data...', width / 2, height / 2);
        
        animationFrameRef.current = requestAnimationFrame(draw);
        return;
      }

      // Get ranges
      const throughputValues = dataHistory.map(d => d.throughput);
      const maxThroughput = Math.max(...throughputValues, 1000);
      const connectionValues = dataHistory.map(d => d.connections);
      const maxConnections = Math.max(...connectionValues, 1);

      // Draw throughput area
      ctx.beginPath();
      dataHistory.forEach((point, index) => {
        const x = (index / (dataHistory.length - 1)) * width;
        const y = height - (point.throughput / maxThroughput) * height * 0.8;
        
        if (index === 0) ctx.moveTo(x, y);
        else ctx.lineTo(x, y);
      });
      
      ctx.lineTo(width, height);
      ctx.lineTo(0, height);
      ctx.closePath();
      
      const gradient = ctx.createLinearGradient(0, 0, 0, height);
      gradient.addColorStop(0, 'rgba(52, 199, 89, 0.3)');
      gradient.addColorStop(1, 'rgba(52, 199, 89, 0.05)');
      ctx.fillStyle = gradient;
      ctx.fill();

      // Draw throughput line
      ctx.beginPath();
      ctx.strokeStyle = '#34C759';
      ctx.lineWidth = 2;
      dataHistory.forEach((point, index) => {
        const x = (index / (dataHistory.length - 1)) * width;
        const y = height - (point.throughput / maxThroughput) * height * 0.8;
        
        if (index === 0) ctx.moveTo(x, y);
        else ctx.lineTo(x, y);
      });
      ctx.stroke();

      // Draw connections line
      ctx.beginPath();
      ctx.strokeStyle = '#007AFF';
      ctx.lineWidth = 2;
      ctx.setLineDash([5, 5]);
      dataHistory.forEach((point, index) => {
        const x = (index / (dataHistory.length - 1)) * width;
        const y = height - (point.connections / maxConnections) * height * 0.6;
        
        if (index === 0) ctx.moveTo(x, y);
        else ctx.lineTo(x, y);
      });
      ctx.stroke();
      ctx.setLineDash([]);

      // Current values
      if (dataHistory.length > 0) {
        const latest = dataHistory[dataHistory.length - 1];
        
        ctx.font = 'bold 18px -apple-system, BlinkMacSystemFont, sans-serif';
        ctx.fillStyle = '#34C759';
        ctx.textAlign = 'left';
        ctx.fillText(formatThroughput(latest.throughput), 15, 25);
        
        ctx.font = '11px -apple-system, BlinkMacSystemFont, sans-serif';
        ctx.fillStyle = 'rgba(52, 199, 89, 0.7)';
        ctx.fillText('Throughput', 15, 40);
        
        ctx.font = 'bold 18px -apple-system, BlinkMacSystemFont, sans-serif';
        ctx.fillStyle = '#007AFF';
        ctx.fillText(`${latest.connections}`, 15, 65);
        
        ctx.font = '11px -apple-system, BlinkMacSystemFont, sans-serif';
        ctx.fillStyle = 'rgba(0, 122, 255, 0.7)';
        ctx.fillText('Connections', 15, 80);
      }

      animationFrameRef.current = requestAnimationFrame(draw);
    };

    resizeCanvas();
    window.addEventListener('resize', resizeCanvas);
    draw();

    return () => {
      isAnimating = false;
      window.removeEventListener('resize', resizeCanvas);
      if (animationFrameRef.current) {
        cancelAnimationFrame(animationFrameRef.current);
      }
    };
  }, [dataHistory]);

  const formatThroughput = (bytes: number): string => {
    if (bytes >= 1024 * 1024 * 1024) return `${(bytes / 1024 / 1024 / 1024).toFixed(1)} GB/s`;
    if (bytes >= 1024 * 1024) return `${(bytes / 1024 / 1024).toFixed(1)} MB/s`;
    if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KB/s`;
    return `${bytes} B/s`;
  };

  return (
    <div className="simple-chart-container">
      <h2 className="section-title">
        <span className="title-icon">📈</span>
        Real-time Performance
      </h2>
      
      <canvas 
        ref={canvasRef}
        className="simple-chart-canvas"
        style={{ width: '100%', height: '180px' }}
      />
    </div>
  );
};