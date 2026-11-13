import { Routes } from '@angular/router';
import { authGuard } from './core/auth.guard';
import { LoginPage } from './pages/login/login';
import { ServersPage } from './pages/servers/servers';

export const routes: Routes = [
  { path: 'login', component: LoginPage },
  { path: 'servers', component: ServersPage, canActivate: [authGuard] },
  { path: '', pathMatch: 'full', redirectTo: 'servers' },
  { path: '**', redirectTo: 'servers' }
];
