import { inject } from '@angular/core';
import { Router } from '@angular/router';
import { catchError, throwError } from 'rxjs';
import { HttpErrorResponse, HttpInterceptorFn } from '@angular/common/http';

function readCookie(name: string): string | null {
  const prefix = `${name}=`;
  const entry = document.cookie.split(';').map((part) => part.trim()).find((part) => part.startsWith(prefix));
  return entry ? decodeURIComponent(entry.slice(prefix.length)) : null;
}

export const csrfInterceptor: HttpInterceptorFn = (request, next) => {
  const router = inject(Router);
  const target = new URL(request.url, window.location.origin);
  if (target.origin !== window.location.origin || !target.pathname.startsWith('/api/')) return next(request);
  let outgoing = request.clone({ withCredentials: true });
  const csrf = readCookie('mmp_csrf');
  if (csrf) {
    outgoing = outgoing.clone({ setHeaders: { 'X-CSRF-Token': csrf } });
  }
  return next(outgoing).pipe(catchError((err: unknown) => {
    if (err instanceof HttpErrorResponse && err.status === 401 && !target.pathname.endsWith('/auth/login')) router.navigate(['/login']);
    return throwError(() => err);
  }));
};
