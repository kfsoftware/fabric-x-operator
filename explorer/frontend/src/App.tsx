import React from "react";
import { Routes, Route, Link, useLocation } from "react-router-dom";
import Dashboard from "./components/Dashboard";
import BlockList from "./components/BlockList";
import BlockDetail from "./components/BlockDetail";
import TxDetail from "./components/TxDetail";
import StateExplorer from "./components/StateExplorer";
import LiveFeed from "./components/LiveFeed";

const navItems = [
  { path: "/", label: "Dashboard" },
  { path: "/blocks", label: "Transactions" },
  { path: "/state", label: "State" },
  { path: "/live", label: "Live" },
];

export default function App() {
  const location = useLocation();

  return (
    <div>
      <nav style={{
        background: "#1a1d27",
        borderBottom: "1px solid #2a2d3a",
        padding: "0.75rem 0",
        marginBottom: "1.5rem",
      }}>
        <div className="container" style={{ display: "flex", alignItems: "center", gap: "2rem" }}>
          <Link to="/" style={{ fontWeight: 700, fontSize: "1.1rem", color: "#e1e4ed" }}>
            Fabric-X Explorer
          </Link>
          <div style={{ display: "flex", gap: "1rem" }}>
            {navItems.map((item) => (
              <Link
                key={item.path}
                to={item.path}
                style={{
                  color: location.pathname === item.path ? "#6366f1" : "#8b8fa3",
                  fontWeight: location.pathname === item.path ? 600 : 400,
                  padding: "0.25rem 0.5rem",
                }}
              >
                {item.label}
              </Link>
            ))}
          </div>
        </div>
      </nav>

      <div className="container">
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/blocks" element={<BlockList />} />
          <Route path="/blocks/:number" element={<BlockDetail />} />
          <Route path="/tx/:txid" element={<TxDetail />} />
          <Route path="/state" element={<StateExplorer />} />
          <Route path="/live" element={<LiveFeed />} />
        </Routes>
      </div>
    </div>
  );
}
