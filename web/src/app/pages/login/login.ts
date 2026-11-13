import { Component, inject } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { Router } from '@angular/router';
import { HttpErrorResponse } from '@angular/common/http';
import { ApiService } from '../../core/api.service';

@Component({
  selector: 'mmp-login',
  standalone: true,
  imports: [FormsModule],
  template: `
    <main class="login-shell">
      <section class="login-card">
        <div class="brand-mark">P</div>
        <p class="eyebrow">INFRASTRUCTURE CONTROL PLANE</p>
        <h1>Multi-Proxmox Manager</h1>
        <p class="muted">Sign in to manage your Proxmox installations.</p>
        <form (ngSubmit)="submit()" #form="ngForm">
          <label for="email">Email</label>
          <input id="email" name="email" type="email" [(ngModel)]="email" autocomplete="username" required maxlength="254">
          <label for="password">Password</label>
          <input id="password" name="password" type="password" [(ngModel)]="password" autocomplete="current-password" required>
          @if (error) { <p class="error-message">{{ error }}</p> }
          <button class="primary-button full-width" type="submit" [disabled]="loading || !form.valid">
            {{ loading ? 'Signing in…' : 'Sign in' }}
          </button>
        </form>
        <p class="footnote">Authorized administrators only.</p>
      </section>
    </main>
  `
})
export class LoginPage {
  private readonly api = inject(ApiService);
  private readonly router = inject(Router);
  email = '';
  password = '';
  loading = false;
  error = '';

  submit(): void {
    this.error = '';
    this.loading = true;
    this.api.login(this.email.trim(), this.password).subscribe({
      next: () => { this.password = ''; this.router.navigate(['/servers']); },
      error: (err: unknown) => {
        this.loading = false;
        this.error = err instanceof HttpErrorResponse && err.status === 429
          ? 'Too many attempts. Try again later.'
          : 'Sign-in failed. Check your email and password.';
      },
      complete: () => { this.loading = false; }
    });
  }
}
