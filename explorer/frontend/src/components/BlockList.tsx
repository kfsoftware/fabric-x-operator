import React, { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { api, type TxList } from "../api/client";

export default function BlockList() {
  const [searchParams, setSearchParams] = useSearchParams();
  const page = parseInt(searchParams.get("page") || "0", 10);
  const limit = 20;
  const [data, setData] = useState<TxList | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    api.transactions(limit, page * limit)
      .then(setData)
      .catch((e) => setError(e.message));
  }, [page]);

  if (error) return <div style={{ color: "#ef4444" }}>Error: {error}</div>;
  if (!data) return <div style={{ color: "#8b8fa3" }}>Loading...</div>;

  const totalPages = Math.ceil(data.total / limit);

  return (
    <div>
      <h1 style={{ fontSize: "1.5rem", marginBottom: "1rem" }}>Transactions</h1>
      <div style={{ color: "#8b8fa3", marginBottom: "1rem" }}>Total: {data.total} transactions</div>

      <div style={{ background: "#1a1d27", border: "1px solid #2a2d3a", borderRadius: "8px", overflowX: "auto" }}>
        <table style={{ width: "100%", borderCollapse: "collapse" }}>
          <thead>
            <tr style={{ borderBottom: "1px solid #2a2d3a", textAlign: "left" }}>
              <th style={{ padding: "0.75rem", color: "#8b8fa3" }}>TX ID</th>
              <th style={{ padding: "0.75rem", color: "#8b8fa3" }}>Block</th>
              <th style={{ padding: "0.75rem", color: "#8b8fa3" }}>Index</th>
              <th style={{ padding: "0.75rem", color: "#8b8fa3" }}>Status</th>
            </tr>
          </thead>
          <tbody>
            {data.transactions?.map((tx) => (
              <tr key={tx.txId} style={{ borderBottom: "1px solid #2a2d3a" }}>
                <td style={{ padding: "0.75rem", fontFamily: "monospace", fontSize: "0.85rem" }}>
                  <Link to={`/tx/${tx.txId}`}>{tx.txId.slice(0, 24)}...</Link>
                </td>
                <td style={{ padding: "0.75rem" }}>
                  <Link to={`/blocks/${tx.blockNum}`} style={{ fontFamily: "monospace" }}>#{tx.blockNum}</Link>
                </td>
                <td style={{ padding: "0.75rem", fontFamily: "monospace" }}>{tx.txNum}</td>
                <td style={{ padding: "0.75rem" }}>
                  <span style={{
                    padding: "2px 8px",
                    borderRadius: "4px",
                    fontSize: "0.8rem",
                    background: tx.statusName === "COMMITTED" ? "#22c55e20" : "#ef444420",
                    color: tx.statusName === "COMMITTED" ? "#22c55e" : "#ef4444",
                    fontWeight: 600,
                  }}>
                    {tx.statusName}
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div style={{ display: "flex", gap: "1rem", marginTop: "1rem", justifyContent: "center" }}>
        <button
          disabled={page <= 0}
          onClick={() => setSearchParams({ page: String(page - 1) })}
          style={{ padding: "0.5rem 1rem", background: "#2a2d3a", color: "#e1e4ed", border: "none", borderRadius: "4px", cursor: page > 0 ? "pointer" : "default", opacity: page > 0 ? 1 : 0.5 }}
        >
          Previous
        </button>
        <span style={{ padding: "0.5rem", color: "#8b8fa3" }}>
          Page {page + 1} of {totalPages || 1}
        </span>
        <button
          disabled={page >= totalPages - 1}
          onClick={() => setSearchParams({ page: String(page + 1) })}
          style={{ padding: "0.5rem 1rem", background: "#2a2d3a", color: "#e1e4ed", border: "none", borderRadius: "4px", cursor: page < totalPages - 1 ? "pointer" : "default", opacity: page < totalPages - 1 ? 1 : 0.5 }}
        >
          Next
        </button>
      </div>
    </div>
  );
}
