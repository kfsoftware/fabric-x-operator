import React, { useEffect, useState } from "react";
import { api, type StateRow, type NamespacesResponse } from "../api/client";

export default function StateExplorer() {
  const [namespaces, setNamespaces] = useState<string[]>([]);
  const [selectedNs, setSelectedNs] = useState("");
  const [rows, setRows] = useState<StateRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    api.namespaces().then((r) => {
      setNamespaces(r.namespaces || []);
      if (r.namespaces?.length) setSelectedNs(r.namespaces[0]);
    }).catch((e) => setError(e.message));
  }, []);

  const fetchState = () => {
    if (!selectedNs) return;
    setLoading(true);
    setError("");
    api.state(selectedNs)
      .then((r) => setRows(r.rows || []))
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false));
  };

  return (
    <div>
      <h1 style={{ fontSize: "1.5rem", marginBottom: "1.5rem" }}>State Explorer</h1>

      <div style={{ display: "flex", gap: "0.75rem", marginBottom: "1.5rem", alignItems: "center" }}>
        <select
          value={selectedNs}
          onChange={(e) => setSelectedNs(e.target.value)}
          style={{
            padding: "0.5rem 0.75rem",
            background: "#1a1d27",
            color: "#e1e4ed",
            border: "1px solid #2a2d3a",
            borderRadius: "4px",
            fontSize: "0.9rem",
          }}
        >
          {namespaces.map((ns) => (
            <option key={ns} value={ns}>{ns}</option>
          ))}
        </select>
        <button
          onClick={fetchState}
          disabled={loading}
          style={{
            padding: "0.5rem 1rem",
            background: "#6366f1",
            color: "white",
            border: "none",
            borderRadius: "4px",
            cursor: "pointer",
          }}
        >
          {loading ? "Loading..." : "Query"}
        </button>
      </div>

      {error && <div style={{ color: "#ef4444", marginBottom: "1rem" }}>{error}</div>}

      {rows.length > 0 && (
        <div style={{ background: "#1a1d27", border: "1px solid #2a2d3a", borderRadius: "8px", overflowX: "auto" }}>
          <table style={{ width: "100%", borderCollapse: "collapse" }}>
            <thead>
              <tr style={{ borderBottom: "1px solid #2a2d3a", textAlign: "left" }}>
                <th style={{ padding: "0.75rem", color: "#8b8fa3" }}>Key</th>
                <th style={{ padding: "0.75rem", color: "#8b8fa3" }}>Version</th>
                <th style={{ padding: "0.75rem", color: "#8b8fa3" }}>Value</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row, i) => (
                <tr key={i} style={{ borderBottom: "1px solid #2a2d3a" }}>
                  <td style={{ padding: "0.75rem" }} className="mono" >{row.key}</td>
                  <td style={{ padding: "0.75rem" }}>{row.version}</td>
                  <td style={{ padding: "0.75rem", wordBreak: "break-all" }} className="mono" >
                    {row.value.slice(0, 60)}{row.value.length > 60 ? "..." : ""}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {rows.length === 0 && !loading && !error && (
        <div style={{ color: "#8b8fa3" }}>Select a namespace and click Query to view state.</div>
      )}
    </div>
  );
}
