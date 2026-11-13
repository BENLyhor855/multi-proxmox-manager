import { Component, OnInit, inject } from '@angular/core';
import { CommonModule, DatePipe } from '@angular/common';
import { FormsModule } from '@angular/forms';
import { Router, RouterLink } from '@angular/router';
import { HttpErrorResponse } from '@angular/common/http';
import { ApiService, Principal, ProxmoxServer, AuditEvent } from '../../core/api.service';

@Component({
  selector: 'mmp-servers',
  standalone: true,
  imports: [CommonModule, DatePipe, FormsModule, RouterLink],
  template: `
    <div class="app-shell">
      <header class="topbar">
        <a class="brand" routerLink="/servers"><span class="brand-mark small">P</span><span>Multi-Proxmox Manager</span></a>
        <div class="account-area">
          @if (user) { <span class="account-label">{{ user.email }} · {{ user.role }}</span> }
          <button class="ghost-button" type="button" (click)="logout()">Sign out</button>
        </div>
      </header>
      <main class="content">
        <div class="page-heading">
          <div>
            <p class="eyebrow">INFRASTRUCTURE</p>
            <h1>Proxmox installations</h1>
            <p class="muted">Manage connections to independent Proxmox VE servers and clusters.</p>
          </div>
          @if (user?.role === 'admin') {
            <button class="primary-button" type="button" (click)="showForm = !showForm">{{ showForm ? 'Close form' : 'Add installation' }}</button>
          }
        </div>

        @if (showForm && user?.role === 'admin') {
          <section class="panel add-panel">
            <h2>Register Proxmox VE</h2>
            <p class="muted small-text">The API token secret is encrypted before storage and is never returned by the API.</p>
            <form class="server-form" (ngSubmit)="addServer()" #serverForm="ngForm">
              <div class="form-grid">
                <div><label for="name">Display name</label><input id="name" name="name" [(ngModel)]="draft.name" required minlength="2" maxlength="80" placeholder="PVE-MAIN"></div>
                <div><label for="location">Location</label><input id="location" name="location" [(ngModel)]="draft.location" maxlength="120" placeholder="Main office"></div>
                <div class="span-2"><label for="url">API URL</label><input id="url" name="url" [(ngModel)]="draft.base_url" required placeholder="https://172.30.0.10:8006"></div>
                <div><label for="tokenId">Token ID</label><input id="tokenId" name="tokenId" [(ngModel)]="draft.token_id" required placeholder="mmp-api@pve!manager"></div>
                <div><label for="tokenSecret">Token secret</label><input id="tokenSecret" name="tokenSecret" type="password" [(ngModel)]="draft.token_secret" required autocomplete="new-password"></div>
                <div class="span-2"><label for="notes">Notes</label><input id="notes" name="notes" [(ngModel)]="draft.notes" maxlength="500" placeholder="Optional notes"></div>
              </div>
              @if (formError) { <p class="error-message">{{ formError }}</p> }
              <div class="form-actions"><button class="primary-button" type="submit" [disabled]="saving || !serverForm.valid">{{ saving ? 'Registering…' : 'Register installation' }}</button></div>
            </form>
          </section>
        }

        @if (message) { <div class="notice">{{ message }}</div> }
        @if (loadError) { <div class="error-banner">{{ loadError }}</div> }

        <section class="stats-grid">
          <article class="stat-card"><span class="stat-label">Installations</span><strong>{{ servers.length }}</strong></article>
          <article class="stat-card"><span class="stat-label">Connected</span><strong>{{ connectedCount }}</strong></article>
          <article class="stat-card"><span class="stat-label">Nodes detected</span><strong>{{ nodeCount }}</strong></article>
        </section>

        <section class="panel">
          <div class="panel-heading"><div><h2>Registered installations</h2><p class="muted small-text">Credentials are not displayed after they are saved.</p></div><button class="ghost-button" type="button" (click)="loadServers()" [disabled]="loading">Refresh</button></div>
          @if (loading) { <p class="muted">Loading installations…</p> }
          @else if (servers.length === 0) { <div class="empty-state"><strong>No Proxmox installations yet</strong><p class="muted">Register your first Proxmox VE endpoint to get started.</p></div> }
          @else {
            <div class="table-wrap">
              <table>
                <thead><tr><th>Name</th><th>Endpoint</th><th>Location</th><th>Version</th><th>Nodes</th><th>Status</th><th>Last check</th><th>Actions</th></tr></thead>
                <tbody>
                  @for (server of servers; track server.id) {
                    <tr>
                      <td><strong>{{ server.name }}</strong><div class="subtext">{{ server.token_id }}</div></td>
                      <td class="mono">{{ server.base_url }}</td>
                      <td>{{ server.location || '—' }}</td>
                      <td>{{ server.version || '—' }}</td>
                      <td>{{ server.node_count }}</td>
                      <td><span class="status" [class.connected]="server.status === 'connected'" [class.disconnected]="server.status === 'disconnected'">{{ server.status }}</span><div class="subtext error-inline">{{ server.last_error }}</div></td>
                      <td>{{ server.last_checked_at ? (server.last_checked_at | date:'medium') : 'Never' }}</td>
                      <td><div class="row-actions">@if (user?.role === 'admin' || user?.role === 'operator') { <button class="small-button" type="button" (click)="test(server)" [disabled]="busyId === server.id">Test</button> }@if (user?.role === 'admin') { <button class="small-button danger" type="button" (click)="remove(server)">Remove</button> }</div></td>
                    </tr>
                  }
                </tbody>
              </table>
            </div>
          }
        </section>
        @if (user?.role === 'admin') {
          <section class="panel">
            <div class="panel-heading"><h2>Recent audit events</h2><button class="ghost-button" type="button" (click)="loadAudit()" [disabled]="auditLoading">{{ auditLoading ? 'Loading…' : 'Refresh audit' }}</button></div>
            @if (auditError) { <p class="error-message">{{ auditError }}</p> }
            <div class="table-wrap"><table><thead><tr><th>Time</th><th>Actor</th><th>Action</th><th>Target</th><th>Source IP</th></tr></thead><tbody>
              @for (event of auditEvents; track event.id) { <tr><td>{{ event.created_at | date:'medium' }}</td><td>{{ event.actor_email || 'Anonymous / system' }}</td><td>{{ event.action }}</td><td>{{ event.target_id || '—' }}</td><td>{{ event.source_ip }}</td></tr> }
            </tbody></table></div>
          </section>
        }
        <p class="footnote page-footnote">Phase 1: secure connectivity and inventory foundation. VM lifecycle controls will be enabled in later phases.</p>
      </main>
    </div>
  `
})
export class ServersPage implements OnInit {
  private readonly api = inject(ApiService);
  private readonly router = inject(Router);
  user: Principal | null = null;
  servers: ProxmoxServer[] = [];
  auditEvents: AuditEvent[] = [];
  auditLoading = false;
  auditError = '';
  loading = false;
  saving = false;
  showForm = false;
  busyId = '';
  message = '';
  formError = '';
  loadError = '';
  draft = { name: '', base_url: '', token_id: '', token_secret: '', location: '', notes: '' };

  get connectedCount(): number { return this.servers.filter((s) => s.status === 'connected').length; }
  get nodeCount(): number { return this.servers.reduce((sum, s) => sum + s.node_count, 0); }

  ngOnInit(): void {
    this.api.me().subscribe({ next: (response) => { this.user = response.user; if (this.user.role === 'admin') this.loadAudit(); }, error: () => this.router.navigate(['/login']) });
    this.loadServers();
  }

  loadAudit(): void {
    this.auditLoading = true;
    this.auditError = '';
    this.api.auditLogs().subscribe({
      next: response => { this.auditEvents = response.items; this.auditLoading = false; },
      error: () => { this.auditLoading = false; this.auditError = 'Unable to load audit events.'; }
    });
  }

  loadServers(): void {
    this.loading = true;
    this.loadError = '';
    this.api.servers().subscribe({
      next: (response) => this.servers = response.items,
      error: () => { this.loading = false; this.loadError = 'Unable to load Proxmox installations. Check the backend connection.'; },
      complete: () => this.loading = false
    });
  }

  addServer(): void {
    this.saving = true;
    this.formError = '';
    this.message = '';
    this.api.createServer(this.draft).subscribe({
      next: (response) => {
        this.message = response.item.status === 'connected'
          ? `Registered ${response.item.name}. Connection successful; ${response.item.node_count} node(s) detected.`
          : `Registered ${response.item.name}, but the initial connection failed: ${response.item.last_error}`;
        this.draft = { name: '', base_url: '', token_id: '', token_secret: '', location: '', notes: '' };
        this.showForm = false;
        this.loadServers();
      },
      error: (err: unknown) => {
        this.formError = err instanceof HttpErrorResponse ? (err.error?.error ?? 'Registration failed.') : 'Registration failed.';
        this.saving = false;
      },
      complete: () => this.saving = false
    });
  }

  test(server: ProxmoxServer): void {
    this.busyId = server.id;
    this.message = '';
    this.api.testServer(server.id).subscribe({
      next: (response) => {
        this.message = `Connection successful: ${server.name} · ${response.connection.node_count} node(s) · Proxmox ${response.connection.version}.`;
        this.loadServers();
        this.busyId = '';
      },
      error: (err: unknown) => {
        const detail = err instanceof HttpErrorResponse ? (err.error?.error ?? 'connection test failed') : 'connection test failed';
        this.message = `${server.name}: ${detail}`;
        this.loadServers();
        this.busyId = '';
      }
    });
  }

  remove(server: ProxmoxServer): void {
    if (!window.confirm(`Remove ${server.name} from this management platform? This does not delete anything from Proxmox VE.`)) return;
    this.api.deleteServer(server.id).subscribe({
      next: () => { this.message = `${server.name} was removed from the manager.`; this.loadServers(); },
      error: () => this.message = 'Unable to remove the installation.'
    });
  }

  logout(): void {
    this.api.logout().subscribe({ next: () => this.router.navigate(['/login']), error: () => { this.message = 'Sign out failed. Please retry.'; } });
  }
}
