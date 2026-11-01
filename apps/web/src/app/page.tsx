'use client';

import { useState } from 'react';

const nav = [
  ['▦', 'Overview'], ['⚡', 'Dispatch', '/dispatch'], ['◫', 'Orders'], ['⌁', 'Tracking'], ['♙', 'Customers'], ['◈', 'Pricing'],
];
const orders = [
  ['SD-20481', 'Amali Fernando', 'Colombo → Kandy', 'In transit', 'LKR 550', 'Today, 10:42'],
  ['SD-20480', 'Nimal Perera', 'Galle → Colombo', 'Delivered', 'LKR 720', 'Today, 09:18'],
  ['SD-20479', 'Sahan Stores', 'Negombo → Kurunegala', 'Out for delivery', 'LKR 480', 'Yesterday, 17:30'],
  ['SD-20478', 'Dinithi Silva', 'Colombo → Matara', 'Created', 'LKR 680', 'Yesterday, 15:05'],
];

export default function Index() {
  const [active, setActive] = useState('Overview');
  const [menu, setMenu] = useState(false);
  return (
    <div className="appShell">
      <aside className={menu ? 'sidebar sidebarOpen' : 'sidebar'}>
        <div className="brand"><b>S</b><span>Swift<i>Drop</i></span></div>
        <p className="workspaceLabel">WORKSPACE</p>
        <div className="workspaceSelect"><b>S</b> SwiftDrop HQ <span>⌄</span></div>
        <nav aria-label="Main navigation">
          {nav.map(([icon, label, href]) => (
            href ? (
              <a key={label} href={href} className="navItem">
                <em>{icon}</em>{label}
              </a>
            ) : (
              <button key={label} className={active === label ? 'navItem navActive' : 'navItem'} onClick={() => { setActive(label); setMenu(false); }}>
                <em>{icon}</em>{label}
              </button>
            )
          ))}
        </nav>
        <div className="sidebarBottom"><button className="navItem"><em>⚙</em>Settings</button><button className="navItem"><em>?</em>Help center</button><div className="profile"><b>RJ</b><span><strong>Ruwan Jayasuriya</strong><small>Administrator</small></span><i>•••</i></div></div>
      </aside>
      <main className="mainContent">
        <header className="topbar"><button className="mobileMenu" onClick={() => setMenu(!menu)} aria-label="Open navigation">☰</button><div className="breadcrumbs">Workspace <b>/</b> <strong>{active}</strong></div><div className="topActions"><button aria-label="Notifications">♢<i /></button><b>RJ</b></div></header>
        <div className="contentWrap">
          <section className="pageIntro"><div><p className="eyebrow">SATURDAY, NOVEMBER 1, 2026</p><h1>Welcome back, Ruwan <span>✦</span></h1><p>Here&apos;s what&apos;s happening with your deliveries today.</p></div><button className="primaryButton">＋ Create order</button></section>
          <section className="metrics"><Metric icon="◫" tone="purple" title="Total orders" value="2,481" change="+12.8%" detail="vs. last month"/><Metric icon="⌁" tone="blue" title="In transit" value="184" change="+4.2%" detail="vs. yesterday"/><Metric icon="✓" tone="green" title="Delivered" value="2,156" change="+8.6%" detail="vs. last month"/><Metric icon="◈" tone="amber" title="Revenue" value="LKR 1.24M" change="+14.1%" detail="vs. last month"/></section>
          <section className="gridMain"><div className="card performanceCard"><Header title="Delivery performance" subtitle="Orders completed over the last 30 days"><button className="selectButton">Last 30 days ⌄</button></Header><div className="chartArea"><div className="chartLabels"><span>300</span><span>200</span><span>100</span><span>0</span></div><div className="chart"><div/><div/><div/><svg viewBox="0 0 640 190" preserveAspectRatio="none" aria-label="Delivery performance"><defs><linearGradient id="fill" x1="0" x2="0" y1="0" y2="1"><stop offset="0" stopColor="#7257e8" stopOpacity=".2"/><stop offset="1" stopColor="#7257e8" stopOpacity="0"/></linearGradient></defs><path d="M0 145 C40 148 55 119 85 130 S125 93 155 112 S190 117 215 83 S252 98 278 70 S315 86 338 61 S374 72 400 50 S432 73 460 44 S497 57 520 35 S555 54 580 25 S616 31 640 12 V190 H0Z" fill="url(#fill)"/><path d="M0 145 C40 148 55 119 85 130 S125 93 155 112 S190 117 215 83 S252 98 278 70 S315 86 338 61 S374 72 400 50 S432 73 460 44 S497 57 520 35 S555 54 580 25 S616 31 640 12" fill="none" stroke="#7257e8" strokeWidth="3" vectorEffect="non-scaling-stroke"/></svg><div className="xLabels"><span>Oct 3</span><span>Oct 10</span><span>Oct 17</span><span>Oct 24</span><span>Oct 31</span></div></div></div></div><div className="card serviceCard"><Header title="Service health" subtitle="Live platform status"><span className="liveBadge">● Operational</span></Header>{['Identity service', 'Parcel operations', 'Pricing service', 'Message queue'].map((name) => <div className="serviceRow" key={name}><span>● {name}</span><strong>99.99%</strong></div>)}<button className="textButton">View system status →</button></div></section>
          <section className="card ordersCard"><Header title="Recent orders" subtitle="The latest delivery activity across your workspace"><button className="outlineButton" onClick={() => setActive('Orders')}>View all orders →</button></Header><div className="tableWrap"><table><thead><tr>{['ORDER', 'CUSTOMER', 'ROUTE', 'STATUS', 'AMOUNT', 'CREATED', ''].map((h) => <th key={h}>{h}</th>)}</tr></thead><tbody>{orders.map((row) => <tr key={row[0]}><td><strong className="orderId">{row[0]}</strong></td><td>{row[1]}</td><td className="muted">{row[2]}</td><td><span className={`status ${row[3].replaceAll(' ', '')}`}><i/> {row[3]}</span></td><td><strong>{row[4]}</strong></td><td className="muted">{row[5]}</td><td><button className="rowMore">•••</button></td></tr>)}</tbody></table></div></section>
          <section className="bottomGrid">
            <div className="card quickCard">
              <Header title="Quick actions" subtitle="Common tasks at your fingertips"/>
              <div className="quickActions">
                <a href="/dispatch" className="quickAction">
                  <b>⚡</b>
                  <span>
                    <strong>Dispatch Center</strong>
                    <small>Assign couriers &amp; monitor</small>
                  </span>
                  <i>→</i>
                </a>
                <button className="quickAction">
                  <b>＋</b>
                  <span>
                    <strong>Create order</strong>
                    <small>Start a new delivery</small>
                  </span>
                  <i>→</i>
                </button>
                <button className="quickAction">
                  <b>⌁</b>
                  <span>
                    <strong>Track parcel</strong>
                    <small>Find a delivery</small>
                  </span>
                  <i>→</i>
                </button>
              </div>
            </div>
            <div className="card noticeCard"><b>✦</b><div><p className="eyebrow">SWIFTDROP INSIGHTS</p><h2>Keep your customers in the loop</h2><p>Set up automated status notifications to reduce “where is my order?” questions.</p><button className="textButton">Explore notifications →</button></div></div>
          </section>
        </div>
      </main>
    </div>
  );
}

function Header({ title, subtitle, children }: { title: string; subtitle: string; children?: React.ReactNode }) { return <div className="cardHeader"><div><h2>{title}</h2><p>{subtitle}</p></div>{children}</div>; }
function Metric({ icon, tone, title, value, change, detail }: { icon: string; tone: string; title: string; value: string; change: string; detail: string }) { return <div className="metric card"><span className={`metricIcon ${tone}`}>{icon}</span><div><p>{title}</p><strong>{value}</strong><small><b>↗ {change}</b> {detail}</small></div></div>; }
