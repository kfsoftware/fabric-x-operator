import React from "react";
import { Link } from "react-router-dom";
import { useSSE } from "../hooks/useSSE";
import type { TxRecord } from "../api/client";

export default function LiveFeed() {
  const { events, connected } = useSSE("/api/v1/events", 100);

  return (
    <div>
      <div style={{ display: "flex", alignItems: "center", gap: "1rem", marginBottom: "1.5rem" }}>
        <h1 style={{ fontSize: "1.5rem" }}>Live Feed</h1>
        <span style={{
          display: "inline-flex",
          alignItems: "center",
          gap: "0.4rem",
          fontSize: "0.8rem",
          color: connected ? "#22c55e" : "#ef4444",
        }}>
          <span style={{
            width: "8px",
            height: "8px",
            borderRadius: "50%",
            background: connected ? "#22c55e" : "#ef4444",
          }} />
          {connected ? "Connected" : "Disconnected"}
        </span>
      </div>

      <div style={{ display: "flex", flexDirection: "column", gap: "0.5rem" }}>
        {events.map((event, i) => {
          if (event.type === "connected") {
            const data = event.data as { height: number };
            return (
              <div key={i} style={{
                background: "#1a1d27",
                border: "1px solid #2a2d3a",
                borderRadius: "8px",
                padding: "0.75rem 1rem",
              }}>
                <span style={{ color: "#8b8fa3" }}>Connected at height </span>
                <span style={{ fontFamily: "monospace", color: "#6366f1" }}>{data.height}</span>
              </div>
            );
          }

          if (event.type === "block") {
            const block = event.data as { number: number; txCount: number; transactions?: TxRecord[] };
            return (
              <div key={i} style={{
                background: "#1a1d27",
                border: "1px solid #2a2d3a",
                borderRadius: "8px",
                padding: "0.75rem 1rem",
              }}>
                <div style={{ display: "flex", alignItems: "center", gap: "1rem", marginBottom: "0.5rem" }}>
                  <Link to={`/blocks/${block.number}`} style={{ fontFamily: "monospace", fontWeight: 600 }}>
                    Block #{block.number}
                  </Link>
                  <span style={{ color: "#8b8fa3", fontSize: "0.85rem" }}>
                    {block.txCount} tx{block.txCount !== 1 ? "s" : ""}
                  </span>
                </div>
                {block.transactions?.map((tx, ti) => (
                  <div key={ti} style={{ paddingLeft: "1rem", fontSize: "0.85rem" }}>
                    <Link to={`/tx/${tx.txId}`} style={{ fontFamily: "monospace" }}>
                      {tx.txId.slice(0, 20)}...
                    </Link>
                    <span style={{
                      marginLeft: "0.5rem",
                      padding: "1px 6px",
                      borderRadius: "3px",
                      fontSize: "0.75rem",
                      background: tx.statusName === "COMMITTED" ? "#22c55e20" : "#ef444420",
                      color: tx.statusName === "COMMITTED" ? "#22c55e" : "#ef4444",
                    }}>
                      {tx.statusName}
                    </span>
                  </div>
                ))}
              </div>
            );
          }

          if (event.type === "error") {
            return (
              <div key={i} style={{
                background: "#1a1d27",
                border: "1px solid #2a2d3a",
                borderRadius: "8px",
                padding: "0.75rem 1rem",
                color: "#ef4444",
              }}>
                Error: {String(event.data)}
              </div>
            );
          }

          return null;
        })}

        {events.length === 0 && (
          <div style={{ color: "#8b8fa3", padding: "2rem", textAlign: "center" }}>
            Waiting for new blocks...
          </div>
        )}
      </div>
    </div>
  );
}
