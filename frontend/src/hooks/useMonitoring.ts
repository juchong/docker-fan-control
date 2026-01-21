import { useState, useEffect } from 'react';
import { wsService } from '../services/websocket';
import { Monitoring } from '../types/monitoring';

export function useMonitoring() {
  const [data, setData] = useState<Monitoring | null>(null);
  const [isConnected, setIsConnected] = useState(false);

  useEffect(() => {
    const unsubscribe = wsService.subscribe((metrics) => {
      setData(metrics);
      setIsConnected(true);
    });

    // Check connection status periodically
    const interval = setInterval(() => {
      setIsConnected(wsService.isConnected);
    }, 1000);

    return () => {
      unsubscribe();
      clearInterval(interval);
    };
  }, []);

  return { data, isConnected };
}
