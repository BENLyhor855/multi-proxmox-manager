import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';

export interface Principal {
  id: number;
  email: string;
  role: 'admin' | 'operator' | 'viewer';
}

export interface ProxmoxServer {
  id: string;
  name: string;
  base_url: string;
  token_id: string;
  location: string;
  notes: string;
  status: 'unknown' | 'connected' | 'disconnected';
  version: string;
  node_count: number;
  last_checked_at: string | null;
  last_error: string;
  created_at: string;
}

export interface AuditEvent {
 id: number; actor_email: string; action: string; target_type: string; target_id: string; source_ip: string; created_at: string;
}

export interface ConnectionResult {
  version: string;
  release: string;
  node_count: number;
  nodes: Array<{ node: string; status: string; cpu?: number; maxcpu?: number; mem?: number; maxmem?: number }>;
}

const API = '/api/v1';

@Injectable({ providedIn: 'root' })
export class ApiService {
  private readonly http = inject(HttpClient);

  login(email: string, password: string): Observable<{ user: Principal }> {
    return this.http.post<{ user: Principal }>(`${API}/auth/login`, { email, password });
  }

  me(): Observable<{ user: Principal }> {
    return this.http.get<{ user: Principal }>(`${API}/auth/me`);
  }

  auditLogs(): Observable<{ items: AuditEvent[] }> {
    return this.http.get<{ items: AuditEvent[] }>(`${API}/audit-logs?limit=100`);
  }

  logout(): Observable<{ status: string }> {
    return this.http.post<{ status: string }>(`${API}/auth/logout`, {});
  }

  servers(): Observable<{ items: ProxmoxServer[] }> {
    return this.http.get<{ items: ProxmoxServer[] }>(`${API}/proxmox/servers`);
  }

  createServer(payload: {
    name: string; base_url: string; token_id: string; token_secret: string; location: string; notes: string;
  }): Observable<{ item: ProxmoxServer; connection: ConnectionResult }> {
    return this.http.post<{ item: ProxmoxServer; connection: ConnectionResult }>(`${API}/proxmox/servers`, payload);
  }

  testServer(id: string): Observable<{ status: string; connection: ConnectionResult }> {
    return this.http.post<{ status: string; connection: ConnectionResult }>(`${API}/proxmox/servers/${id}/test`, {});
  }

  deleteServer(id: string): Observable<{ status: string }> {
    return this.http.delete<{ status: string }>(`${API}/proxmox/servers/${id}`);
  }
}
