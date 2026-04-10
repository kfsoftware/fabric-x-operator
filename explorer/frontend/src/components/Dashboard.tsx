import React, { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api, type Dashboard as DashboardData } from "../api/client";

const cardStyle: React.CSSProperties = {
  background: "#1a1d27",
  border: "1px solid #2a2d3a",
  borderRadius: "8px",
  padding: "1.25rem",
};

export default function Dashboard() {
  const [data, setData] = useState<DashboardData | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    api.dashboard().then(setData).catch((e) => setError(e.message));
    const id = setInterval(() => {
      api.dashboard().then(setData).catch(() => {});
    }, 5000);
    return () => clearInterval(id);
  }, []);

  if (error) return <div style={{ color: "#ef4444" }}>Error: {error}</div>;
  if (!data) return <div style={{ color: "#8b8fa3" }}>Loading...</div>;

  return (
    <div>
      <h1 style={{ fontSize: "1.5rem", marginBottom: "1.5rem" }}>Dashboard</h1>

      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit, minmax(200px, 1fr))", gap: "1rem", marginBottom: "2rem" }}>
        <div style={cardStyle}>
          <div style={{ color: "#8b8fa3", fontSize: "0.8rem", textTransform: "uppercase", letterSpacing: "0.05em" }}>Blockchain Height</div>
          <div style={{ fontSize: "2rem", fontWeight: 700, color: "#6366f1" }}>{data.height}</div>
        </div>
        <div style={cardStyle}>
          <div style={{ color: "#8b8fa3", fontSize: "0.8rem", textTransform: "uppercase", letterSpacing: "0.05em" }}>Total Blocks</div>
          <div style={{ fontSize: "2rem", fontWeight: 700 }}>{data.totalBlocks}</div>
        </div>
        <div style={cardStyle}>
          <div style={{ color: "#8b8fa3", fontSize: "0.8rem", textTransform: "uppercase", letterSpacing: "0.05em" }}>Total Transactions</div>
          <div style={{ fontSize: "2rem", fontWeight: 700 }}>{data.totalTxs}</div>
        </div>
      </div>

      <h2 style={{ fontSize: "1.1rem", marginBottom: "1rem" }}>Recent Transactions</h2>
      <div style={{ ...cardStyle, overflowX: "auto" }}>
        <table style={{ width: "100%", borderCollapse: "collapse" }}>
          <thead>
            <tr style={{ borderBottom: "1px solid #2a2d3a", textAlign: "left" }}>
              <th style={{ padding: "0.5rem", color: "#8b8fa3", fontWeight: 500 }}>TX ID</th>
              <th style={{ padding: "0.5rem", color: "#8b8fa3", fontWeight: 500 }}>Block</th>
              <th style={{ padding: "0.5rem", color: "#8b8fa3", fontWeight: 500 }}>Status</th>
            </tr>
          </thead>
          <tbody>
            {data.recentTxs?.map((tx) => (
              <tr key={tx.txId} style={{ borderBottom: "1px solid #2a2d3a" }}>
                <td style={{ padding: "0.5rem" }}>
                  <Link to={`/tx/${tx.txId}`} style={{ fontFamily: "monospace", fontSize: "0.85rem" }}>
                    {tx.txId.slice(0, 20)}...
                  </Link>
                </td>
                <td style={{ padding: "0.5rem" }}>
                  <Link to={`/blocks/${tx.blockNum}`} style={{ fontFamily: "monospace" }}>
                    #{tx.blockNum}
                  </Link>
                </td>
                <td style={{ padding: "0.5rem" }}>
                  <StatusBadge status={tx.statusName} />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

function StatusBadge({ status }: { status: string }) {
  const color = status === "COMMITTED" ? "#22c55e"
    : status.startsWith("ABORTED") ? "#ef4444"
    : "#f59e0b";
  return (
    <span style={{
      padding: "2px 8px",
      borderRadius: "4px",
      fontSize: "0.8rem",
      background: color + "20",
      color: color,
      fontWeight: 600,
    }}>
      {status}
    </span>
  );
}
