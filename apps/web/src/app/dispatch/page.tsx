'use client';

import React, { useState, useMemo } from 'react';

// Type definitions matching API v1.1 contracts
type UserRole = 'DISPATCHER' | 'ADMIN' | 'COURIER' | 'CUSTOMER';
type ParcelSizeCategory = 'SMALL' | 'MEDIUM' | 'LARGE' | 'OVERSIZED';
type ParcelLifecycleStatus =
  | 'CREATED'
  | 'PICKED_UP'
  | 'IN_TRANSIT'
  | 'OUT_FOR_DELIVERY'
  | 'DELIVERED'
  | 'FAILED';

interface UnassignedParcel {
  id: string;
  trackingNumber: string;
  pickupCity: string;
  dropoffCity: string;
  weightKg: number;
  sizeCategory: ParcelSizeCategory;
  createdAt: string;
  pickupZoneId: string;
  zoneName: string;
}

interface CourierWorkload {
  courierId: string;
  fullName: string;
  phone: string;
  isAvailable: boolean;
  activeDeliveries: number;
  breakdown: {
    CREATED: number;
    PICKED_UP: number;
    IN_TRANSIT: number;
    OUT_FOR_DELIVERY: number;
  };
}

interface MonitoredParcel {
  id: string;
  trackingNumber: string;
  status: ParcelLifecycleStatus;
  pickupCity: string;
  dropoffCity: string;
  zone: string;
  courierName: string | null;
  updatedAt: string;
}

// Initial Mock Data (aligned with API v1.1 specs)
const INITIAL_UNASSIGNED: UnassignedParcel[] = [
  {
    id: 'p-101',
    trackingNumber: 'SDK7M2Q9XP4',
    pickupCity: 'Colombo',
    dropoffCity: 'Kandy',
    weightKg: 1.5,
    sizeCategory: 'MEDIUM',
    createdAt: 'Today, 08:30 AM',
    pickupZoneId: 'zone-colombo',
    zoneName: 'Western (Colombo)',
  },
  {
    id: 'p-102',
    trackingNumber: 'SDN4T8W2LC6',
    pickupCity: 'Nugegoda',
    dropoffCity: 'Galle',
    weightKg: 0.8,
    sizeCategory: 'SMALL',
    createdAt: 'Today, 09:15 AM',
    pickupZoneId: 'zone-colombo',
    zoneName: 'Western (Colombo)',
  },
  {
    id: 'p-103',
    trackingNumber: 'SDK2P9L1TR8',
    pickupCity: 'Kandy',
    dropoffCity: 'Matale',
    weightKg: 4.2,
    sizeCategory: 'LARGE',
    createdAt: 'Today, 09:40 AM',
    pickupZoneId: 'zone-kandy',
    zoneName: 'Central (Kandy)',
  },
  {
    id: 'p-104',
    trackingNumber: 'SDN9X3K5MN2',
    pickupCity: 'Galle',
    dropoffCity: 'Matara',
    weightKg: 12.0,
    sizeCategory: 'OVERSIZED',
    createdAt: 'Today, 10:05 AM',
    pickupZoneId: 'zone-galle',
    zoneName: 'Southern (Galle)',
  },
];

const INITIAL_COURIERS: CourierWorkload[] = [
  {
    courierId: 'c-01',
    fullName: 'Kamal Perera',
    phone: '+94 77 123 4567',
    isAvailable: true,
    activeDeliveries: 4,
    breakdown: { CREATED: 1, PICKED_UP: 1, IN_TRANSIT: 1, OUT_FOR_DELIVERY: 1 },
  },
  {
    courierId: 'c-02',
    fullName: 'Sunil Jayamaha',
    phone: '+94 71 987 6543',
    isAvailable: true,
    activeDeliveries: 1,
    breakdown: { CREATED: 0, PICKED_UP: 0, IN_TRANSIT: 0, OUT_FOR_DELIVERY: 1 },
  },
  {
    courierId: 'c-03',
    fullName: 'Nilmini Silva',
    phone: '+94 76 555 4321',
    isAvailable: true,
    activeDeliveries: 0,
    breakdown: { CREATED: 0, PICKED_UP: 0, IN_TRANSIT: 0, OUT_FOR_DELIVERY: 0 },
  },
  {
    courierId: 'c-04',
    fullName: 'Kasun Fernando',
    phone: '+94 72 333 8899',
    isAvailable: false,
    activeDeliveries: 5,
    breakdown: { CREATED: 2, PICKED_UP: 1, IN_TRANSIT: 1, OUT_FOR_DELIVERY: 1 },
  },
];

const INITIAL_MONITORED_PARCELS: MonitoredParcel[] = [
  {
    id: 'm-1',
    trackingNumber: 'SDK7M2Q9XP4',
    status: 'CREATED',
    pickupCity: 'Colombo',
    dropoffCity: 'Kandy',
    zone: 'Western (Colombo)',
    courierName: null,
    updatedAt: '10 mins ago',
  },
  {
    id: 'm-2',
    trackingNumber: 'SDN4T8W2LC6',
    status: 'CREATED',
    pickupCity: 'Nugegoda',
    dropoffCity: 'Galle',
    zone: 'Western (Colombo)',
    courierName: null,
    updatedAt: '25 mins ago',
  },
  {
    id: 'm-3',
    trackingNumber: 'SDK9J1B4QQ9',
    status: 'PICKED_UP',
    pickupCity: 'Colombo',
    dropoffCity: 'Negombo',
    zone: 'Western (Colombo)',
    courierName: 'Kamal Perera',
    updatedAt: '5 mins ago',
  },
  {
    id: 'm-4',
    trackingNumber: 'SDN2L8K7VV3',
    status: 'IN_TRANSIT',
    pickupCity: 'Kandy',
    dropoffCity: 'Colombo',
    zone: 'Central (Kandy)',
    courierName: 'Kamal Perera',
    updatedAt: '12 mins ago',
  },
  {
    id: 'm-5',
    trackingNumber: 'SDK5M3X9WW1',
    status: 'OUT_FOR_DELIVERY',
    pickupCity: 'Colombo',
    dropoffCity: 'Colombo 07',
    zone: 'Western (Colombo)',
    courierName: 'Sunil Jayamaha',
    updatedAt: '2 mins ago',
  },
  {
    id: 'm-6',
    trackingNumber: 'SDN8R4C2ZZ8',
    status: 'DELIVERED',
    pickupCity: 'Galle',
    dropoffCity: 'Matara',
    zone: 'Southern (Galle)',
    courierName: 'Kasun Fernando',
    updatedAt: '1 hour ago',
  },
  {
    id: 'm-7',
    trackingNumber: 'SDK1W9N5YY4',
    status: 'FAILED',
    pickupCity: 'Colombo',
    dropoffCity: 'Dehiwala',
    zone: 'Western (Colombo)',
    courierName: 'Kasun Fernando',
    updatedAt: '2 hours ago',
  },
];

export default function DispatcherDashboardPage() {
  // Role Guard State (DISPATCHER and ADMIN permitted)
  const [currentRole, setCurrentRole] = useState<UserRole>('DISPATCHER');
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
  const [activeSection, setActiveSection] = useState<
    'all' | 'unassigned' | 'workload' | 'monitor'
  >('all');

  // Interactive Data States
  const [unassignedList, setUnassignedList] =
    useState<UnassignedParcel[]>(INITIAL_UNASSIGNED);
  const [couriers, setCouriers] = useState<CourierWorkload[]>(INITIAL_COURIERS);
  const [monitoredParcels, setMonitoredParcels] = useState<MonitoredParcel[]>(
    INITIAL_MONITORED_PARCELS,
  );

  // Modal State for Assigning Courier
  const [selectedParcel, setSelectedParcel] = useState<UnassignedParcel | null>(
    null,
  );
  const [assignedCourierId, setAssignedCourierId] = useState<string>('');
  const [assignReason, setAssignReason] = useState<string>('');
  const [toastMessage, setToastMessage] = useState<string | null>(null);

  // Search & Filter Bar State (GET /api/v1/admin/parcels)
  const [searchTracking, setSearchTracking] = useState('');
  const [selectedStatus, setSelectedStatus] = useState<string>('ALL');
  const [selectedZone, setSelectedZone] = useState<string>('ALL');

  // Trigger Toast Notification
  const showToast = (msg: string) => {
    setToastMessage(msg);
    setTimeout(() => setToastMessage(null), 4000);
  };

  // Preview only: simulates PUT /api/v1/dispatch/parcels/{parcelId}/assignment
  const handleConfirmAssignment = (e: React.FormEvent) => {
    e.preventDefault();
    if (!selectedParcel || !assignedCourierId) return;

    const courier = couriers.find((c) => c.courierId === assignedCourierId);
    if (!courier) return;

    // Remove from unassigned queue
    setUnassignedList((prev) => prev.filter((p) => p.id !== selectedParcel.id));

    // Update courier active workload
    setCouriers((prev) =>
      prev.map((c) =>
        c.courierId === assignedCourierId
          ? {
              ...c,
              activeDeliveries: c.activeDeliveries + 1,
              breakdown: { ...c.breakdown, CREATED: c.breakdown.CREATED + 1 },
            }
          : c,
      ),
    );

    // Update or add to parcel monitor list
    setMonitoredParcels((prev) => [
      {
        id: selectedParcel.id,
        trackingNumber: selectedParcel.trackingNumber,
        status: 'CREATED',
        pickupCity: selectedParcel.pickupCity,
        dropoffCity: selectedParcel.dropoffCity,
        zone: selectedParcel.zoneName,
        courierName: courier.fullName,
        updatedAt: 'Just now',
      },
      ...prev.filter((p) => p.trackingNumber !== selectedParcel.trackingNumber),
    ]);

    showToast(
      `Assigned ${selectedParcel.trackingNumber} to ${courier.fullName} successfully!`,
    );
    setSelectedParcel(null);
    setAssignedCourierId('');
    setAssignReason('');
  };

  // Filtered Parcels for All Parcels Monitor
  const filteredMonitoredParcels = useMemo(() => {
    return monitoredParcels.filter((p) => {
      const matchTracking =
        searchTracking === '' ||
        p.trackingNumber.toLowerCase().includes(searchTracking.toLowerCase());
      const matchStatus =
        selectedStatus === 'ALL' || p.status === selectedStatus;
      const matchZone = selectedZone === 'ALL' || p.zone.includes(selectedZone);
      return matchTracking && matchStatus && matchZone;
    });
  }, [monitoredParcels, searchTracking, selectedStatus, selectedZone]);

  // Role Guard Check: Only DISPATCHER and ADMIN allowed
  const isAuthorized = currentRole === 'DISPATCHER' || currentRole === 'ADMIN';

  return (
    <div className="appShell">
      {/* Sidebar Navigation */}
      <aside className={mobileMenuOpen ? 'sidebar sidebarOpen' : 'sidebar'}>
        <div className="brand">
          <b>S</b>
          <span>
            Swift<i>Drop</i>
          </span>
        </div>
        <p className="workspaceLabel">LOGISTICS DISPATCH</p>
        <div className="workspaceSelect">
          <b>⚡</b> Central Operations <span>⌄</span>
        </div>

        <nav aria-label="Dispatcher Navigation">
          <button
            className={`navItem ${activeSection === 'all' ? 'navActive' : ''}`}
            onClick={() => {
              setActiveSection('all');
              setMobileMenuOpen(false);
            }}
          >
            <em>▦</em> Dispatch Dashboard
          </button>
          <button
            className={`navItem ${activeSection === 'unassigned' ? 'navActive' : ''}`}
            onClick={() => {
              setActiveSection('unassigned');
              setMobileMenuOpen(false);
            }}
          >
            <em>◫</em> Unassigned Queue ({unassignedList.length})
          </button>
          <button
            className={`navItem ${activeSection === 'workload' ? 'navActive' : ''}`}
            onClick={() => {
              setActiveSection('workload');
              setMobileMenuOpen(false);
            }}
          >
            <em>👥</em> Courier Workload
          </button>
          <button
            className={`navItem ${activeSection === 'monitor' ? 'navActive' : ''}`}
            onClick={() => {
              setActiveSection('monitor');
              setMobileMenuOpen(false);
            }}
          >
            <em>⌁</em> Parcels Monitor
          </button>
          <a href="/" className="navItem" style={{ marginTop: '12px' }}>
            <em>←</em> Back to Main App
          </a>
        </nav>

        <div className="sidebarBottom">
          <div
            style={{
              padding: '8px 12px',
              background: '#f5f3ff',
              borderRadius: '8px',
              marginBottom: '10px',
            }}
          >
            <span
              style={{
                fontSize: '10px',
                color: '#6d28d9',
                fontWeight: 700,
                textTransform: 'uppercase',
              }}
            >
              Active Role
            </span>
            <select
              aria-label="Simulated user role"
              value={currentRole}
              onChange={(e) => setCurrentRole(e.target.value as UserRole)}
              style={{
                width: '100%',
                marginTop: '4px',
                padding: '6px',
                borderRadius: '6px',
                border: '1px solid #d8b4fe',
                fontSize: '11px',
                fontWeight: 600,
                color: '#4c1d95',
                background: '#fff',
              }}
            >
              <option value="DISPATCHER">DISPATCHER (Authorized)</option>
              <option value="ADMIN">ADMIN (Authorized)</option>
              <option value="COURIER">COURIER (Restricted)</option>
              <option value="CUSTOMER">CUSTOMER (Restricted)</option>
            </select>
          </div>
          <div className="profile">
            <b>{currentRole === 'ADMIN' ? 'AD' : 'DP'}</b>
            <span>
              <strong>Ruwan Jayasuriya</strong>
              <small>{currentRole} Session</small>
            </span>
            <i>•••</i>
          </div>
        </div>
      </aside>

      {/* Main Operations Area */}
      <main className="mainContent">
        {/* Top bar */}
        <header className="topbar">
          <button
            className="mobileMenu"
            onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
            aria-label="Toggle navigation"
          >
            ☰
          </button>
          <div className="breadcrumbs">
            Dispatch Center <b>/</b>{' '}
            <strong>
              {activeSection === 'all' ? 'Operations Overview' : activeSection}
            </strong>
          </div>
          <div className="topActions">
            <span
              style={{
                fontSize: '11px',
                color: isAuthorized ? '#15803d' : '#b91c1c',
                fontWeight: 600,
              }}
            >
              ● {isAuthorized ? 'Guard: Authorized' : 'Guard: Forbidden (403)'}
            </span>
            <b>{currentRole[0]}</b>
          </div>
        </header>

        {/* Toast Notification */}
        {toastMessage && (
          <div
            style={{
              position: 'fixed',
              top: '84px',
              right: '24px',
              zIndex: 9999,
              background: '#15803d',
              color: '#fff',
              padding: '12px 20px',
              borderRadius: '8px',
              boxShadow: '0 4px 12px rgba(0,0,0,0.15)',
              fontSize: '13px',
              fontWeight: 600,
              display: 'flex',
              alignItems: 'center',
              gap: '8px',
            }}
          >
            ✓ {toastMessage}
          </div>
        )}

        {/* Content Wrapper */}
        <div className="contentWrap">
          {/* Role Guard Enforcement Screen */}
          {!isAuthorized ? (
            <section
              className="card"
              style={{
                padding: '40px 24px',
                textAlign: 'center',
                maxWidth: '640px',
                margin: '40px auto',
              }}
            >
              <div
                style={{
                  width: '56px',
                  height: '56px',
                  background: '#fee2e2',
                  color: '#dc2626',
                  borderRadius: '50%',
                  display: 'grid',
                  placeItems: 'center',
                  fontSize: '24px',
                  margin: '0 auto 16px',
                }}
              >
                ✕
              </div>
              <h2
                style={{
                  fontSize: '20px',
                  fontWeight: 800,
                  color: '#991b1b',
                  marginBottom: '8px',
                }}
              >
                403 Forbidden: Access Restricted
              </h2>
              <p
                style={{
                  color: '#6b7280',
                  fontSize: '13px',
                  lineHeight: 1.6,
                  marginBottom: '20px',
                }}
              >
                The Dispatcher Dashboard and Assignment operations are
                restricted to <strong>DISPATCHER</strong> and{' '}
                <strong>ADMIN</strong> roles (API v1.1 Section 5). The currently
                simulated user role{' '}
                <code
                  style={{
                    background: '#f3f4f6',
                    padding: '2px 6px',
                    borderRadius: '4px',
                  }}
                >
                  {currentRole}
                </code>{' '}
                is not authorized to view or manage dispatch queues.
              </p>
              <button
                className="primaryButton"
                onClick={() => setCurrentRole('DISPATCHER')}
                style={{ margin: '0 auto' }}
              >
                Switch Role to DISPATCHER
              </button>
            </section>
          ) : (
            <>
              {/* Dispatch Intro Header */}
              <section className="pageIntro">
                <div>
                  <p className="eyebrow">DISPATCH PREVIEW (API v1.1)</p>
                  <h1>
                    Dispatcher Control Hub <span>⚡</span>
                  </h1>
                  <p>
                    Allocate unassigned parcels, balance courier capacity, and
                    monitor delivery progress in real time.
                  </p>
                  <p
                    style={{
                      fontSize: '12px',
                      color: '#6b7280',
                      marginTop: '6px',
                    }}
                  >
                    Preview UI: the dispatch endpoints are planned and this page
                    uses local demo data.
                  </p>
                </div>
                <div style={{ display: 'flex', gap: '10px' }}>
                  <button
                    className="outlineButton"
                    onClick={() => {
                      showToast(
                        'Queues synchronized with Parcel Operations service.',
                      );
                    }}
                  >
                    ↻ Sync Queues
                  </button>
                </div>
              </section>

              {/* KPI Summary Metrics */}
              <section className="metrics">
                <div className="metric card">
                  <span className="metricIcon amber">⏳</span>
                  <div>
                    <p>Unassigned Parcels</p>
                    <strong>{unassignedList.length}</strong>
                    <small>
                      <b>GET</b> /api/v1/dispatch/unassigned
                    </small>
                  </div>
                </div>
                <div className="metric card">
                  <span className="metricIcon green">🛵</span>
                  <div>
                    <p>Available Couriers</p>
                    <strong>
                      {couriers.filter((c) => c.isAvailable).length} /{' '}
                      {couriers.length}
                    </strong>
                    <small>
                      <b>GET</b> /api/v1/dispatch/workload
                    </small>
                  </div>
                </div>
                <div className="metric card">
                  <span className="metricIcon blue">🚚</span>
                  <div>
                    <p>In Transit</p>
                    <strong>
                      {
                        monitoredParcels.filter(
                          (p) =>
                            p.status === 'IN_TRANSIT' ||
                            p.status === 'OUT_FOR_DELIVERY',
                        ).length
                      }
                    </strong>
                    <small>Active deliveries in field</small>
                  </div>
                </div>
                <div className="metric card">
                  <span className="metricIcon purple">✓</span>
                  <div>
                    <p>Completed Deliveries</p>
                    <strong>
                      {
                        monitoredParcels.filter((p) => p.status === 'DELIVERED')
                          .length
                      }
                    </strong>
                    <small>Today&apos;s finished routes</small>
                  </div>
                </div>
              </section>

              {/* 1. Unassigned Parcels Queue Section */}
              {(activeSection === 'all' || activeSection === 'unassigned') && (
                <section
                  className="card ordersCard"
                  style={{ marginBottom: '24px' }}
                >
                  <div className="cardHeader">
                    <div>
                      <h2>1. Unassigned Parcels Queue</h2>
                      <p>
                        Parcels awaiting courier assignment (
                        <code>GET /api/v1/dispatch/unassigned</code>). Closed
                        parcels cannot be assigned.
                      </p>
                    </div>
                    <span
                      style={{
                        padding: '4px 10px',
                        background: '#fef3c7',
                        color: '#92400e',
                        borderRadius: '6px',
                        fontSize: '11px',
                        fontWeight: 700,
                      }}
                    >
                      {unassignedList.length} PENDING
                    </span>
                  </div>

                  <div className="tableWrap">
                    <table>
                      <thead>
                        <tr>
                          <th>TRACKING #</th>
                          <th>PICKUP CITY</th>
                          <th>DROPOFF CITY</th>
                          <th>WEIGHT</th>
                          <th>SIZE</th>
                          <th>CREATED AT</th>
                          <th>ACTION</th>
                        </tr>
                      </thead>
                      <tbody>
                        {unassignedList.length === 0 ? (
                          <tr>
                            <td
                              colSpan={7}
                              style={{
                                textAlign: 'center',
                                padding: '30px',
                                color: '#9ca3af',
                              }}
                            >
                              All parcels have been assigned to couriers! Good
                              job.
                            </td>
                          </tr>
                        ) : (
                          unassignedList.map((parcel) => (
                            <tr key={parcel.id}>
                              <td>
                                <strong className="orderId">
                                  {parcel.trackingNumber}
                                </strong>
                              </td>
                              <td>{parcel.pickupCity}</td>
                              <td>
                                <strong>{parcel.dropoffCity}</strong>
                              </td>
                              <td className="muted">
                                {parcel.weightKg.toFixed(2)} kg
                              </td>
                              <td>
                                <span
                                  style={{
                                    display: 'inline-block',
                                    padding: '2px 8px',
                                    borderRadius: '4px',
                                    fontSize: '10px',
                                    fontWeight: 700,
                                    background:
                                      parcel.sizeCategory === 'SMALL'
                                        ? '#e0f2fe'
                                        : parcel.sizeCategory === 'MEDIUM'
                                          ? '#ede9fe'
                                          : parcel.sizeCategory === 'LARGE'
                                            ? '#fef3c7'
                                            : '#fee2e2',
                                    color:
                                      parcel.sizeCategory === 'SMALL'
                                        ? '#0369a1'
                                        : parcel.sizeCategory === 'MEDIUM'
                                          ? '#6d28d9'
                                          : parcel.sizeCategory === 'LARGE'
                                            ? '#b45309'
                                            : '#b91c1c',
                                  }}
                                >
                                  {parcel.sizeCategory}
                                </span>
                              </td>
                              <td className="muted">{parcel.createdAt}</td>
                              <td>
                                <button
                                  className="primaryButton"
                                  style={{
                                    padding: '6px 12px',
                                    fontSize: '11px',
                                  }}
                                  onClick={() => {
                                    setSelectedParcel(parcel);
                                    setAssignedCourierId(
                                      couriers.find((c) => c.isAvailable)
                                        ?.courierId || '',
                                    );
                                  }}
                                >
                                  Assign Courier →
                                </button>
                              </td>
                            </tr>
                          ))
                        )}
                      </tbody>
                    </table>
                  </div>
                </section>
              )}

              {/* 2. Courier Workload Panel Section */}
              {(activeSection === 'all' || activeSection === 'workload') && (
                <section
                  className="card"
                  style={{ padding: '22px', marginBottom: '24px' }}
                >
                  <div className="cardHeader" style={{ marginBottom: '16px' }}>
                    <div>
                      <h2>2. Courier Workload Panel</h2>
                      <p>
                        Real-time courier availability and delivery workload
                        breakdown (<code>GET /api/v1/dispatch/workload</code>).
                      </p>
                    </div>
                  </div>

                  <div
                    style={{
                      display: 'grid',
                      gridTemplateColumns:
                        'repeat(auto-fit, minmax(280px, 1fr))',
                      gap: '16px',
                    }}
                  >
                    {couriers.map((courier) => (
                      <div
                        key={courier.courierId}
                        style={{
                          border: '1px solid #ebeaf1',
                          borderRadius: '10px',
                          padding: '16px',
                          background: '#fff',
                        }}
                      >
                        <div
                          style={{
                            display: 'flex',
                            justifyContent: 'space-between',
                            alignItems: 'flex-start',
                            marginBottom: '8px',
                          }}
                        >
                          <div>
                            <strong
                              style={{
                                fontSize: '14px',
                                color: '#1d1b2b',
                                display: 'block',
                              }}
                            >
                              {courier.fullName}
                            </strong>
                            <small
                              style={{ color: '#888597', fontSize: '11px' }}
                            >
                              {courier.phone}
                            </small>
                          </div>
                          <span
                            style={{
                              padding: '3px 8px',
                              borderRadius: '12px',
                              fontSize: '10px',
                              fontWeight: 700,
                              background: courier.isAvailable
                                ? '#dcfce7'
                                : '#f3f4f6',
                              color: courier.isAvailable
                                ? '#15803d'
                                : '#6b7280',
                            }}
                          >
                            ● {courier.isAvailable ? 'Available' : 'Busy / Off'}
                          </span>
                        </div>

                        <div
                          style={{
                            margin: '12px 0 10px',
                            display: 'flex',
                            justifyContent: 'space-between',
                            alignItems: 'center',
                          }}
                        >
                          <span
                            style={{
                              fontSize: '11px',
                              color: '#77758a',
                              fontWeight: 600,
                            }}
                          >
                            Active Deliveries:
                          </span>
                          <strong
                            style={{ fontSize: '16px', color: '#6d28d9' }}
                          >
                            {courier.activeDeliveries}
                          </strong>
                        </div>

                        {/* Status Breakdown Pills */}
                        <div
                          style={{
                            display: 'grid',
                            gridTemplateColumns: 'repeat(4, 1fr)',
                            gap: '6px',
                            textAlign: 'center',
                          }}
                        >
                          <div
                            style={{
                              background: '#f5f3ff',
                              padding: '6px 4px',
                              borderRadius: '6px',
                            }}
                          >
                            <span
                              style={{
                                display: 'block',
                                fontSize: '9px',
                                color: '#7c3aed',
                                fontWeight: 600,
                              }}
                            >
                              CREATED
                            </span>
                            <strong
                              style={{ fontSize: '12px', color: '#4c1d95' }}
                            >
                              {courier.breakdown.CREATED}
                            </strong>
                          </div>
                          <div
                            style={{
                              background: '#fef3c7',
                              padding: '6px 4px',
                              borderRadius: '6px',
                            }}
                          >
                            <span
                              style={{
                                display: 'block',
                                fontSize: '9px',
                                color: '#b45309',
                                fontWeight: 600,
                              }}
                            >
                              PICKED UP
                            </span>
                            <strong
                              style={{ fontSize: '12px', color: '#92400e' }}
                            >
                              {courier.breakdown.PICKED_UP}
                            </strong>
                          </div>
                          <div
                            style={{
                              background: '#e0f2fe',
                              padding: '6px 4px',
                              borderRadius: '6px',
                            }}
                          >
                            <span
                              style={{
                                display: 'block',
                                fontSize: '9px',
                                color: '#0369a1',
                                fontWeight: 600,
                              }}
                            >
                              IN TRANSIT
                            </span>
                            <strong
                              style={{ fontSize: '12px', color: '#075985' }}
                            >
                              {courier.breakdown.IN_TRANSIT}
                            </strong>
                          </div>
                          <div
                            style={{
                              background: '#fef9c3',
                              padding: '6px 4px',
                              borderRadius: '6px',
                            }}
                          >
                            <span
                              style={{
                                display: 'block',
                                fontSize: '9px',
                                color: '#a16207',
                                fontWeight: 600,
                              }}
                            >
                              OUT FOR DEL
                            </span>
                            <strong
                              style={{ fontSize: '12px', color: '#854d0e' }}
                            >
                              {courier.breakdown.OUT_FOR_DELIVERY}
                            </strong>
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                </section>
              )}

              {/* 3. All Parcels Search & Monitor Filter Bar Section */}
              {(activeSection === 'all' || activeSection === 'monitor') && (
                <section className="card ordersCard">
                  <div
                    className="cardHeader"
                    style={{ flexWrap: 'wrap', gap: '12px' }}
                  >
                    <div>
                      <h2>3. All Parcels Search &amp; Monitor</h2>
                      <p>
                        Track and audit parcel lifecycles across all zones and
                        couriers (<code>GET /api/v1/admin/parcels</code>).
                      </p>
                    </div>
                  </div>

                  {/* Filter Bar */}
                  <div
                    style={{
                      display: 'flex',
                      flexWrap: 'wrap',
                      gap: '12px',
                      padding: '0 0 16px',
                      borderBottom: '1px solid #ebeaf1',
                      alignItems: 'center',
                    }}
                  >
                    <div style={{ flex: '1 1 240px' }}>
                      <label
                        style={{
                          display: 'block',
                          fontSize: '10px',
                          fontWeight: 700,
                          color: '#888597',
                          marginBottom: '4px',
                        }}
                      >
                        SEARCH TRACKING NUMBER
                      </label>
                      <input
                        type="text"
                        placeholder="e.g. SDK7M2Q9XP4..."
                        value={searchTracking}
                        onChange={(e) => setSearchTracking(e.target.value)}
                        style={{
                          width: '100%',
                          padding: '8px 12px',
                          border: '1px solid #ebeaf1',
                          borderRadius: '8px',
                          fontSize: '12px',
                          outline: 'none',
                        }}
                      />
                    </div>

                    <div style={{ flex: '1 1 180px' }}>
                      <label
                        style={{
                          display: 'block',
                          fontSize: '10px',
                          fontWeight: 700,
                          color: '#888597',
                          marginBottom: '4px',
                        }}
                      >
                        FILTER STATUS
                      </label>
                      <select
                        value={selectedStatus}
                        onChange={(e) => setSelectedStatus(e.target.value)}
                        style={{
                          width: '100%',
                          padding: '8px 12px',
                          border: '1px solid #ebeaf1',
                          borderRadius: '8px',
                          fontSize: '12px',
                          background: '#fff',
                        }}
                      >
                        <option value="ALL">All Statuses</option>
                        <option value="CREATED">CREATED</option>
                        <option value="PICKED_UP">PICKED_UP</option>
                        <option value="IN_TRANSIT">IN_TRANSIT</option>
                        <option value="OUT_FOR_DELIVERY">
                          OUT_FOR_DELIVERY
                        </option>
                        <option value="DELIVERED">DELIVERED</option>
                        <option value="FAILED">FAILED</option>
                      </select>
                    </div>

                    <div style={{ flex: '1 1 180px' }}>
                      <label
                        style={{
                          display: 'block',
                          fontSize: '10px',
                          fontWeight: 700,
                          color: '#888597',
                          marginBottom: '4px',
                        }}
                      >
                        FILTER ZONE
                      </label>
                      <select
                        value={selectedZone}
                        onChange={(e) => setSelectedZone(e.target.value)}
                        style={{
                          width: '100%',
                          padding: '8px 12px',
                          border: '1px solid #ebeaf1',
                          borderRadius: '8px',
                          fontSize: '12px',
                          background: '#fff',
                        }}
                      >
                        <option value="ALL">All Zones</option>
                        <option value="Western">Western (Colombo)</option>
                        <option value="Central">Central (Kandy)</option>
                        <option value="Southern">Southern (Galle)</option>
                      </select>
                    </div>
                  </div>

                  {/* Parcel Monitor Table */}
                  <div className="tableWrap" style={{ marginTop: '14px' }}>
                    <table>
                      <thead>
                        <tr>
                          <th>TRACKING #</th>
                          <th>STATUS</th>
                          <th>ROUTE</th>
                          <th>OPERATING ZONE</th>
                          <th>ASSIGNED COURIER</th>
                          <th>LAST UPDATED</th>
                        </tr>
                      </thead>
                      <tbody>
                        {filteredMonitoredParcels.length === 0 ? (
                          <tr>
                            <td
                              colSpan={6}
                              style={{
                                textAlign: 'center',
                                padding: '24px',
                                color: '#9ca3af',
                              }}
                            >
                              No parcels match your search and filter criteria.
                            </td>
                          </tr>
                        ) : (
                          filteredMonitoredParcels.map((parcel) => (
                            <tr key={parcel.id}>
                              <td>
                                <strong className="orderId">
                                  {parcel.trackingNumber}
                                </strong>
                              </td>
                              <td>
                                <span
                                  className={`status ${
                                    parcel.status === 'IN_TRANSIT'
                                      ? 'Intransit'
                                      : parcel.status === 'OUT_FOR_DELIVERY'
                                        ? 'Outfordelivery'
                                        : parcel.status === 'DELIVERED'
                                          ? 'Delivered'
                                          : ''
                                  }`}
                                  style={{
                                    color:
                                      parcel.status === 'FAILED'
                                        ? '#dc2626'
                                        : undefined,
                                    background:
                                      parcel.status === 'FAILED'
                                        ? '#fee2e2'
                                        : undefined,
                                  }}
                                >
                                  <i>●</i> {parcel.status}
                                </span>
                              </td>
                              <td className="muted">
                                {parcel.pickupCity} → {parcel.dropoffCity}
                              </td>
                              <td>{parcel.zone}</td>
                              <td>
                                {parcel.courierName ? (
                                  <span
                                    style={{
                                      fontWeight: 600,
                                      color: '#374151',
                                    }}
                                  >
                                    {parcel.courierName}
                                  </span>
                                ) : (
                                  <span
                                    style={{
                                      color: '#d97706',
                                      fontStyle: 'italic',
                                    }}
                                  >
                                    Unassigned
                                  </span>
                                )}
                              </td>
                              <td className="muted">{parcel.updatedAt}</td>
                            </tr>
                          ))
                        )}
                      </tbody>
                    </table>
                  </div>
                </section>
              )}
            </>
          )}
        </div>
      </main>

      {/* Assign Courier Modal / Drawer (API 5.2 PUT /dispatch/parcels/{parcelId}/assignment) */}
      {selectedParcel && (
        <div
          style={{
            position: 'fixed',
            inset: 0,
            background: 'rgba(29, 27, 43, 0.6)',
            display: 'grid',
            placeItems: 'center',
            zIndex: 10000,
            padding: '16px',
          }}
          onClick={(e) => {
            if (e.target === e.currentTarget) setSelectedParcel(null);
          }}
        >
          <div
            style={{
              background: '#fff',
              borderRadius: '14px',
              maxWidth: '480px',
              width: '100%',
              padding: '24px',
              boxShadow: '0 20px 25px -5px rgba(0, 0, 0, 0.1)',
            }}
          >
            <div
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'center',
                marginBottom: '16px',
              }}
            >
              <div>
                <h3
                  style={{
                    margin: 0,
                    fontSize: '18px',
                    fontWeight: 800,
                    color: '#1d1b2b',
                  }}
                >
                  Assign Courier to Parcel
                </h3>
                <small style={{ color: '#77758a' }}>
                  {
                    'API v1.1 Endpoint: PUT /api/v1/dispatch/parcels/{parcelId}/assignment'
                  }
                </small>
              </div>
              <button
                onClick={() => setSelectedParcel(null)}
                style={{
                  background: 'none',
                  fontSize: '20px',
                  color: '#9ca3af',
                  cursor: 'pointer',
                }}
              >
                ✕
              </button>
            </div>

            <div
              style={{
                background: '#f8f8fb',
                padding: '14px',
                borderRadius: '8px',
                marginBottom: '16px',
              }}
            >
              <div
                style={{
                  display: 'flex',
                  justifyContent: 'space-between',
                  marginBottom: '6px',
                }}
              >
                <span style={{ fontSize: '11px', color: '#6b7280' }}>
                  Tracking Number:
                </span>
                <strong style={{ fontSize: '12px', color: '#6d28d9' }}>
                  {selectedParcel.trackingNumber}
                </strong>
              </div>
              <div
                style={{
                  display: 'flex',
                  justifyContent: 'space-between',
                  marginBottom: '6px',
                }}
              >
                <span style={{ fontSize: '11px', color: '#6b7280' }}>
                  Route:
                </span>
                <span style={{ fontSize: '12px', fontWeight: 600 }}>
                  {selectedParcel.pickupCity} → {selectedParcel.dropoffCity}
                </span>
              </div>
              <div style={{ display: 'flex', justifyContent: 'space-between' }}>
                <span style={{ fontSize: '11px', color: '#6b7280' }}>
                  Package Specs:
                </span>
                <span style={{ fontSize: '11px', color: '#374151' }}>
                  {selectedParcel.weightKg} kg · {selectedParcel.sizeCategory}
                </span>
              </div>
            </div>

            <form onSubmit={handleConfirmAssignment}>
              <div style={{ marginBottom: '14px' }}>
                <label
                  style={{
                    display: 'block',
                    fontSize: '11px',
                    fontWeight: 700,
                    color: '#4b5563',
                    marginBottom: '6px',
                  }}
                >
                  SELECT AVAILABLE COURIER *
                </label>
                <select
                  required
                  value={assignedCourierId}
                  onChange={(e) => setAssignedCourierId(e.target.value)}
                  style={{
                    width: '100%',
                    padding: '10px',
                    borderRadius: '8px',
                    border: '1px solid #d1d5db',
                    fontSize: '13px',
                    background: '#fff',
                  }}
                >
                  <option value="" disabled>
                    Choose a field courier...
                  </option>
                  {couriers.map((c) => (
                    <option key={c.courierId} value={c.courierId}>
                      {c.fullName} ({c.activeDeliveries} active deliveries) —{' '}
                      {c.isAvailable ? 'Available' : 'Busy'}
                    </option>
                  ))}
                </select>
              </div>

              <div style={{ marginBottom: '20px' }}>
                <label
                  style={{
                    display: 'block',
                    fontSize: '11px',
                    fontWeight: 700,
                    color: '#4b5563',
                    marginBottom: '6px',
                  }}
                >
                  ASSIGNMENT / REASSIGNMENT REASON (OPTIONAL)
                </label>
                <input
                  type="text"
                  placeholder="e.g. Route optimization for Western Zone"
                  value={assignReason}
                  onChange={(e) => setAssignReason(e.target.value)}
                  style={{
                    width: '100%',
                    padding: '10px',
                    borderRadius: '8px',
                    border: '1px solid #d1d5db',
                    fontSize: '12px',
                  }}
                />
              </div>

              <div
                style={{
                  display: 'flex',
                  justifyContent: 'flex-end',
                  gap: '10px',
                }}
              >
                <button
                  type="button"
                  className="outlineButton"
                  onClick={() => setSelectedParcel(null)}
                >
                  Cancel
                </button>
                <button type="submit" className="primaryButton">
                  Confirm Assignment
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
