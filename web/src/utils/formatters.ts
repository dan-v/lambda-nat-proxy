export const formatBytes = (bytes: number): string => {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return (bytes / Math.pow(k, i)).toFixed(1) + ' ' + sizes[i];
};

export const formatDuration = (ms: number): string => {
  const seconds = Math.floor(ms / 1000);
  const minutes = Math.floor(seconds / 60);
  const hours = Math.floor(minutes / 60);
  const days = Math.floor(hours / 24);

  if (days > 0) {
    return `${days}d ${hours % 24}h`;
  } else if (hours > 0) {
    return `${hours}h ${minutes % 60}m`;
  } else if (minutes > 0) {
    return `${minutes}m ${seconds % 60}s`;
  } else {
    return `${seconds}s`;
  }
};

export const formatLatency = (ms: number): string => {
  if (ms < 1) return '<1ms';
  return `${Math.round(ms)}ms`;
};

export const formatUptime = (uptime: string): string => {
  // Parse uptime string and convert to clean format
  const match = uptime.match(/(?:(\d+)h)?(?:(\d+)m)?(?:(\d+(?:\.\d+)?)s)?/);
  if (!match) return uptime;
  
  const [, hours, minutes, seconds] = match;
  const parts = [];
  
  if (hours && parseInt(hours) > 0) parts.push(`${hours}h`);
  if (minutes && parseInt(minutes) > 0) parts.push(`${minutes}m`);
  if (seconds && !hours) {
    // Only show seconds if no hours, and round to whole seconds
    parts.push(`${Math.round(parseFloat(seconds))}s`);
  }
  
  if (parts.length === 0) return '0s';
  return parts.join(' ');
};