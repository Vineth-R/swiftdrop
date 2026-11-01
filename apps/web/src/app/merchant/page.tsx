'use client';

import { useState } from 'react';

const orders = [
  ['SZ-100245', 'SDN4T8W2LC6', 'Amali Fernando', 'Kandy', 'In transit', 'LKR 550'],
  ['SZ-100244', 'SDN1P7A9QK2', 'Nimal Perera', 'Colombo', 'Delivered', 'LKR 720'],
  ['SZ-100243', 'SDN8M3C5RV1', 'Dinithi Silva', 'Matara', 'Created', 'LKR 680'],
];

export default function MerchantPage() {
  const [section, setSection] = useState('Overview');
  return (
    <main className="merchantPage">
      <header className="merchantHeader"><a className="merchantLogo" href="/">S <span>SwiftDrop</span></a><nav><button className={section === 'Overview' ? 'merchantNavActive' : ''} onClick={() => setSection('Overview')}>Overview</button><button className={section === 'Orders' ? 'merchantNavActive' : ''} onClick={() => setSection('Orders')}>Orders</button><button onClick={() => setSection('Profile')}>Business profile</button><button onClick={() => setSection('API keys')}>API keys</button></nav><div className="merchantUser"><span>RJ</span> Ruwan <b>⌄</b></div></header>
      <div className="merchantBody"><div className="merchantWelcome"><div><p className="eyebrow">MERCHANT PORTAL</p><h1>{section === 'Overview' ? 'Welcome back, ShopZone' : section}</h1><p>Manage your deliveries and keep customers moving.</p></div><button className="primaryButton">＋ Create order</button></div>
        {section === 'Overview' && <><div className="merchantStatus"><span className="approvedDot"/> Account approved <small>Last synced just now</small></div><div className="merchantMetrics"><MerchantMetric label="Total orders" value="248"/><MerchantMetric label="In transit" value="18"/><MerchantMetric label="Delivered" value="214"/><MerchantMetric label="This month" value="LKR 124,850"/></div><div className="merchantColumns"><section className="merchantPanel"><div className="merchantPanelHead"><div><h2>Recent orders</h2><p>Your latest delivery requests</p></div><button className="textButton" onClick={() => setSection('Orders')}>View all →</button></div><div className="merchantOrderList">{orders.map((order) => <div className="merchantOrder" key={order[0]}><div className="orderBadge">{order[0].slice(-2)}</div><div><strong>{order[0]}</strong><small>{order[2]} · {order[3]}</small></div><span className={`status ${order[4].replaceAll(' ', '')}`}>{order[4]}</span><b>{order[5]}</b></div>)}</div></section><section className="merchantPanel setupPanel"><p className="eyebrow">GET STARTED</p><h2>Connect your store</h2><p>Use an API key to send orders from your ecommerce platform to SwiftDrop.</p><button className="outlineButton" onClick={() => setSection('API keys')}>Manage API keys →</button><div className="setupProgress"><span/><span/><span/></div><small>2 of 3 setup steps complete</small></section></div></>}
        {section === 'Orders' && <section className="merchantPanel"><div className="merchantPanelHead"><div><h2>Merchant orders</h2><p>Orders submitted by your store</p></div><button className="outlineButton">Filter ▾</button></div><div className="merchantTableWrap"><table><thead><tr><th>EXTERNAL ORDER</th><th>TRACKING</th><th>RECIPIENT</th><th>STATUS</th><th>FEE</th></tr></thead><tbody>{orders.concat(orders).map((order, i) => <tr key={`${order[0]}-${i}`}><td><strong>{order[0]}</strong></td><td>{order[1]}</td><td>{order[2]}<small>{order[3]}</small></td><td><span className={`status ${order[4].replaceAll(' ', '')}`}>{order[4]}</span></td><td><strong>{order[5]}</strong></td></tr>)}</tbody></table></div></section>}
        {section === 'Profile' && <ProfilePanel />}
        {section === 'API keys' && <ApiPanel />}
      </div>
    </main>
  );
}

function MerchantMetric({ label, value }: { label: string; value: string }) { return <div className="merchantMetric"><span>{label}</span><strong>{value}</strong><small>↗ 8.4% <em>this month</em></small></div>; }
function ProfilePanel() { return <section className="merchantPanel formPanel"><div className="merchantPanelHead"><div><h2>Business profile</h2><p>Keep your merchant details up to date.</p></div><span className="status Delivered">APPROVED</span></div><div className="merchantForm"><label>Business name<input defaultValue="ShopZone Lanka" /></label><label>Registration number<input defaultValue="PV 123456" /></label><label>Contact email<input defaultValue="support@shopzone.lk" type="email" /></label><label>Contact phone<input defaultValue="+94 11 222 3344" /></label><label className="wide">Pickup address<input defaultValue="88 Galle Road, Colombo 03" /></label></div><button className="primaryButton">Save changes</button></section>; }
function ApiPanel() { return <section className="merchantPanel"><div className="merchantPanelHead"><div><h2>API keys</h2><p>Use these keys to connect your store to SwiftDrop.</p></div><button className="primaryButton">＋ Create API key</button></div><div className="apiNotice">⌁ <span>Keep your secret safe. The full key is shown only once when it is created.</span></div><div className="apiKeyRow"><div className="keyIcon">⌘</div><div><strong>Production</strong><small>sdk_live_ab12•••••••• · Created today</small></div><span className="status Delivered">ACTIVE</span><button className="rowMore">•••</button></div></section>; }
