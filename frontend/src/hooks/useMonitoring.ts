import { useEffect, useState } from 'react';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { wsService } from '../services/websocket';
import { monitoringApi } from '../services/api';
import { Monitoring } from '../types/monitoring';

// Single shared monitoring source: the WebSocket feeds the ['monitoring'] query
// cache (so every useMonitoring() call — Dashboard, Fans, Profiles, Header —
// reads the same data instead of holding 4 independent copies), and REST polling
// runs ONLY while the socket is down, as a fallback so the UI never freezes.
export function useMonitoring() {
  const queryClient = useQueryClient();
  const [isConnected, setIsConnected] = useState(wsService.isConnected);

  const query = useQuery<Monitoring>({
    queryKey: ['monitoring'],
    queryFn: () => monitoringApi.getMetrics() as Promise<Monitoring>,
    // Poll via REST only when the websocket isn't delivering frames.
    refetchInterval: () => (wsService.isConnected ? false : 2000),
    staleTime: 0,
  });

  useEffect(() => {
    const unsubscribe = wsService.subscribe((metrics) => {
      queryClient.setQueryData(['monitoring'], metrics);
      setIsConnected(true);
    });
    const interval = setInterval(() => setIsConnected(wsService.isConnected), 1000);
    return () => {
      unsubscribe();
      clearInterval(interval);
    };
  }, [queryClient]);

  return { data: query.data ?? null, isConnected };
}
