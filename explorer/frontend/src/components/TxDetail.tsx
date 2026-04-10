import React, { useEffect, useState } from "react";
import { useParams, Link } from "react-router-dom";
import { api, type TxDetail as TxDetailData } from "../api/client";

const sectionStyle: React.CSSProperties = {
  background: "#1a1d27",
  border: "1px solid #2a2d3a",
  borderRadius: "8px",
  padding: "1rem",
  marginBottom: "1.5rem",
};

const labelStyle: React.CSSProperties = { padding: "0.5rem", color: "#8b8fa3", width: "130px" };
const cellStyle: React.CSSProperties = { padding: "0.5rem", wordBreak: "break-all" };

export default function TxDetail() {
  const { txid } = useParams();
  const [tx, setTx] = useState<TxDetailData | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    if (txid) {
      api.tx(txid).then(setTx).catch((e: Error) => setError(e.message));
    }
  }, [txid]);

  if (error) return <div style={{ color: "#ef4444" }}>Error: {error}</div>;
  if (!tx) return <div style={{ color: "#8b8fa3" }}>Loading...</div>;

  const statusColor = tx.statusName === "COMMITTED" ? "#22c55e"
    : tx.statusName.startsWith("ABORTED") ? "#ef4444"
    : "#f59e0b";

  return (
    <div>
      <h1 style={{ fontSize: "1.5rem", marginBottom: "1.5rem" }}>Transaction</h1>

      <div style={sectionStyle}>
        <table style={{ width: "100%", borderCollapse: "collapse" }}>
          <tbody>
            <tr style={{ borderBottom: "1px solid #2a2d3a" }}>
              <td style={labelStyle}>TX ID</td>
              <td style={{ ...cellStyle, fontFamily: "monospace", fontSize: "0.85rem" }}>{tx.txId}</td>
            </tr>
            <tr style={{ borderBottom: "1px solid #2a2d3a" }}>
              <td style={labelStyle}>Block</td>
              <td style={cellStyle}>
                <Link to={`/blocks/${tx.blockNum}`} style={{ fontFamily: "monospace" }}>#{tx.blockNum}</Link>
              </td>
            </tr>
            <tr style={{ borderBottom: "1px solid #2a2d3a" }}>
              <td style={labelStyle}>TX Index</td>
              <td style={{ ...cellStyle, fontFamily: "monospace" }}>{tx.txNum}</td>
            </tr>
            {tx.type && (
              <tr style={{ borderBottom: "1px solid #2a2d3a" }}>
                <td style={labelStyle}>Type</td>
                <td style={cellStyle}>
                  <span style={{
                    padding: "2px 8px", borderRadius: "4px", fontSize: "0.8rem",
                    background: tx.type === "application" ? "#1e3a5f" : "#3a2e1e",
                    color: tx.type === "application" ? "#60a5fa" : "#f59e0b",
                  }}>{tx.type}</span>
                </td>
              </tr>
            )}
            {tx.channelId && (
              <tr style={{ borderBottom: "1px solid #2a2d3a" }}>
                <td style={labelStyle}>Channel</td>
                <td style={cellStyle}>{tx.channelId}</td>
              </tr>
            )}
            {tx.timestamp && (
              <tr style={{ borderBottom: "1px solid #2a2d3a" }}>
                <td style={labelStyle}>Timestamp</td>
                <td style={cellStyle}>{new Date(tx.timestamp).toLocaleString()}</td>
              </tr>
            )}
            <tr>
              <td style={labelStyle}>Status</td>
              <td style={cellStyle}>
                <span style={{
                  padding: "2px 8px", borderRadius: "4px", fontSize: "0.8rem",
                  background: statusColor + "20", color: statusColor, fontWeight: 600,
                }}>{tx.statusName} ({tx.status})</span>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      {tx.endorsers && tx.endorsers.length > 0 && (
        <>
          <h2 style={{ fontSize: "1.1rem", marginBottom: "0.75rem" }}>Endorsers</h2>
          <div style={sectionStyle}>
            {tx.endorsers.map((e, i) => (
              <div key={i} style={{ padding: "0.25rem 0", display: "flex", gap: "1rem" }}>
                <span style={{ fontWeight: 600, color: "#6366f1" }}>{e.mspId}</span>
                {e.subject && <span style={{ color: "#8b8fa3" }}>{e.subject}</span>}
              </div>
            ))}
          </div>
        </>
      )}

      {tx.namespaces?.map((ns, ni) => (
        <div key={ni}>
          <h2 style={{ fontSize: "1.1rem", marginBottom: "0.75rem" }}>
            Namespace: <span style={{ fontFamily: "monospace", color: "#6366f1" }}>{ns.nsId}</span>
            <span style={{ color: "#8b8fa3", fontSize: "0.8rem", marginLeft: "0.5rem" }}>v{ns.nsVersion}</span>
          </h2>
          <div style={sectionStyle}>
            {ns.reads && ns.reads.length > 0 && (
              <div style={{ marginBottom: "1rem" }}>
                <h3 style={{ fontSize: "0.9rem", color: "#60a5fa", marginBottom: "0.5rem" }}>
                  Reads ({ns.reads.length})
                </h3>
                {ns.reads.map((r, i) => (
                  <div key={i} style={{ padding: "0.4rem 0", borderBottom: "1px solid #2a2d3a" }}>
                    {r.keyLabel && (
                      <div style={{ fontSize: "0.85rem", color: "#60a5fa", fontWeight: 600, marginBottom: "0.2rem" }}>
                        {r.keyLabel}
                      </div>
                    )}
                    <div style={{ fontFamily: "monospace", fontSize: "0.75rem", color: "#8b8fa3" }}>
                      key: {r.key} &nbsp; version: {r.version ?? "nil"}
                    </div>
                  </div>
                ))}
              </div>
            )}
            {ns.readWrites && ns.readWrites.length > 0 && (
              <div style={{ marginBottom: "1rem" }}>
                <h3 style={{ fontSize: "0.9rem", color: "#22c55e", marginBottom: "0.5rem" }}>
                  Read-Writes ({ns.readWrites.length})
                </h3>
                {ns.readWrites.map((rw, i) => (
                  <div key={i} style={{ padding: "0.5rem 0", borderBottom: "1px solid #2a2d3a" }}>
                    {rw.keyLabel && (
                      <div style={{ fontSize: "0.85rem", color: "#22c55e", fontWeight: 600, marginBottom: "0.2rem" }}>
                        {rw.keyLabel}
                      </div>
                    )}
                    <div style={{ fontFamily: "monospace", fontSize: "0.75rem", color: "#8b8fa3", marginBottom: "0.2rem" }}>
                      key: {rw.key} &nbsp; version: {rw.version ?? "nil"}
                    </div>
                    {rw.valueInfo && (
                      <div style={{ fontSize: "0.8rem", color: "#e1e4ed", paddingLeft: "0.5rem", marginBottom: "0.2rem" }}>
                        {rw.valueInfo}
                      </div>
                    )}
                    <details style={{ paddingLeft: "0.5rem" }}>
                      <summary style={{ fontSize: "0.75rem", color: "#8b8fa3", cursor: "pointer" }}>
                        Raw value ({rw.value.length / 2} bytes)
                      </summary>
                      <div style={{
                        fontFamily: "monospace", fontSize: "0.7rem", color: "#22c55e",
                        marginTop: "0.3rem", wordBreak: "break-all",
                        maxHeight: "6rem", overflow: "auto",
                        background: "#0f1117", padding: "0.5rem", borderRadius: "4px",
                      }}>
                        {rw.value}
                      </div>
                    </details>
                  </div>
                ))}
              </div>
            )}
            {ns.blindWrites && ns.blindWrites.length > 0 && (
              <div>
                <h3 style={{ fontSize: "0.9rem", color: "#f59e0b", marginBottom: "0.5rem" }}>
                  Blind Writes ({ns.blindWrites.length})
                </h3>
                {ns.blindWrites.map((w, i) => (
                  <div key={i} style={{ padding: "0.5rem 0", borderBottom: "1px solid #2a2d3a" }}>
                    {w.keyLabel && (
                      <div style={{ fontSize: "0.85rem", color: "#f59e0b", fontWeight: 600, marginBottom: "0.2rem" }}>
                        {w.keyLabel}
                      </div>
                    )}
                    <div style={{ fontFamily: "monospace", fontSize: "0.75rem", color: "#8b8fa3", marginBottom: "0.2rem" }}>
                      key: {w.key}
                    </div>
                    {w.valueInfo && (
                      <div style={{ fontSize: "0.8rem", color: "#e1e4ed", paddingLeft: "0.5rem", marginBottom: "0.2rem" }}>
                        {w.valueInfo}
                      </div>
                    )}
                    <details style={{ paddingLeft: "0.5rem" }}>
                      <summary style={{ fontSize: "0.75rem", color: "#8b8fa3", cursor: "pointer" }}>
                        Raw value ({w.value.length / 2} bytes)
                      </summary>
                      <div style={{
                        fontFamily: "monospace", fontSize: "0.7rem", color: "#f59e0b",
                        marginTop: "0.3rem", wordBreak: "break-all",
                        maxHeight: "6rem", overflow: "auto",
                        background: "#0f1117", padding: "0.5rem", borderRadius: "4px",
                      }}>
                        {w.value}
                      </div>
                    </details>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      ))}
    </div>
  );
}
