import React, { useEffect, useState } from "react";
import { useParams, Link } from "react-router-dom";
import { api, type DecodedBlock } from "../api/client";

export default function BlockDetail() {
  const { number } = useParams();
  const [block, setBlock] = useState<DecodedBlock | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    if (number) {
      api.block(parseInt(number, 10)).then(setBlock).catch((e: Error) => setError(e.message));
    }
  }, [number]);

  if (error) return <div style={{ color: "#ef4444" }}>Error: {error}</div>;
  if (!block) return <div style={{ color: "#8b8fa3" }}>Loading...</div>;

  return (
    <div>
      <h1 style={{ fontSize: "1.5rem", marginBottom: "1.5rem" }}>
        Block #{block.number}
      </h1>

      <div style={{ background: "#1a1d27", border: "1px solid #2a2d3a", borderRadius: "8px", padding: "1rem", marginBottom: "2rem" }}>
        <table style={{ width: "100%", borderCollapse: "collapse" }}>
          <tbody>
            <tr style={{ borderBottom: "1px solid #2a2d3a" }}>
              <td style={{ padding: "0.5rem", color: "#8b8fa3", width: "140px" }}>Number</td>
              <td style={{ padding: "0.5rem", fontFamily: "monospace" }}>{block.number}</td>
            </tr>
            <tr style={{ borderBottom: "1px solid #2a2d3a" }}>
              <td style={{ padding: "0.5rem", color: "#8b8fa3" }}>Transactions</td>
              <td style={{ padding: "0.5rem" }}>{block.txCount}</td>
            </tr>
            <tr style={{ borderBottom: "1px solid #2a2d3a" }}>
              <td style={{ padding: "0.5rem", color: "#8b8fa3" }}>Data Hash</td>
              <td style={{ padding: "0.5rem", fontFamily: "monospace", fontSize: "0.8rem", wordBreak: "break-all" }}>{block.dataHash}</td>
            </tr>
            <tr>
              <td style={{ padding: "0.5rem", color: "#8b8fa3" }}>Previous Hash</td>
              <td style={{ padding: "0.5rem", fontFamily: "monospace", fontSize: "0.8rem", wordBreak: "break-all" }}>{block.previousHash}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <h2 style={{ fontSize: "1.1rem", marginBottom: "1rem" }}>Transactions</h2>
      <div style={{ background: "#1a1d27", border: "1px solid #2a2d3a", borderRadius: "8px", overflowX: "auto" }}>
        <table style={{ width: "100%", borderCollapse: "collapse" }}>
          <thead>
            <tr style={{ borderBottom: "1px solid #2a2d3a", textAlign: "left" }}>
              <th style={{ padding: "0.75rem", color: "#8b8fa3" }}>#</th>
              <th style={{ padding: "0.75rem", color: "#8b8fa3" }}>TX ID</th>
              <th style={{ padding: "0.75rem", color: "#8b8fa3" }}>Type</th>
              <th style={{ padding: "0.75rem", color: "#8b8fa3" }}>Timestamp</th>
            </tr>
          </thead>
          <tbody>
            {block.transactions?.map((tx) => (
              <tr key={tx.index} style={{ borderBottom: "1px solid #2a2d3a" }}>
                <td style={{ padding: "0.75rem" }}>{tx.index}</td>
                <td style={{ padding: "0.75rem", fontFamily: "monospace", fontSize: "0.85rem" }}>
                  {tx.txId ? (
                    <Link to={`/tx/${tx.txId}`}>{tx.txId.slice(0, 20)}...</Link>
                  ) : (
                    <span style={{ color: "#8b8fa3" }}>-</span>
                  )}
                </td>
                <td style={{ padding: "0.75rem" }}>
                  <span style={{
                    padding: "2px 8px", borderRadius: "4px", fontSize: "0.8rem",
                    background: tx.type === "application" ? "#1e3a5f" : "#3a2e1e",
                    color: tx.type === "application" ? "#60a5fa" : "#f59e0b",
                  }}>{tx.type}</span>
                </td>
                <td style={{ padding: "0.75rem", color: "#8b8fa3", fontSize: "0.85rem" }}>
                  {tx.timestamp ? new Date(tx.timestamp).toLocaleString() : "-"}
                </td>
              </tr>
            ))}
            {(!block.transactions || block.transactions.length === 0) && (
              <tr>
                <td colSpan={4} style={{ padding: "1rem", color: "#8b8fa3", textAlign: "center" }}>
                  No transactions in this block
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
