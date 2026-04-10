import { useEffect, useRef, useState } from "react";

export interface SSEEvent {
  type: string;
  data: unknown;
}

export function useSSE(url: string, maxEvents = 50) {
  const [events, setEvents] = useState<SSEEvent[]>([]);
  const [connected, setConnected] = useState(false);
  const esRef = useRef<EventSource | null>(null);

  useEffect(() => {
    const es = new EventSource(url);
    esRef.current = es;

    es.onmessage = (e) => {
      const event: SSEEvent = JSON.parse(e.data);
      if (event.type === "connected") {
        setConnected(true);
      }
      setEvents((prev) => [event, ...prev].slice(0, maxEvents));
    };

    es.onerror = () => {
      setConnected(false);
    };

    return () => {
      es.close();
      esRef.current = null;
    };
  }, [url, maxEvents]);

  return { events, connected };
}
