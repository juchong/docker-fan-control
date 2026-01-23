import { useState, useEffect, useRef } from 'react';
import { wsService } from '../services/websocket';
import { monitoringApi } from '../services/api';
import { Monitoring } from '../types/monitoring';

export function useMonitoring() {
  const [data, setData] = useState<Monitoring | null>(null);
  const [isConnected, setIsConnected] = useState(false);
  const initialFetchDone = useRef(false);

  useEffect(() => {
    // Fetch initial data via REST immediately (don't wait for websocket)
    if (!initialFetchDone.current) {
      initialFetchDone.current = true;
      monitoringApi.getMetrics()
        .then((metrics) => {
          // Only set if we don't have data yet (websocket might have connected first)
          setData((current) => current ?? metrics as Monitoring);
        })
        .catch((err) => {
          console.error('Failed to fetch initial metrics:', err);
        });
    }

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
