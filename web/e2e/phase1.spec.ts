import { test, expect, Page } from '@playwright/test';
const password = process.env['E2E_PASSWORD'];
test.skip(!password, 'Set E2E_PASSWORD for disposable admin/operator/viewer@example.com accounts');
async function login(page: Page, role: string) {
 await page.goto('/login');
 await page.getByLabel('Email', { exact: true }).fill(`${role}@example.com`);
 await page.getByLabel('Password', { exact: true }).fill(password!);
 await page.getByRole('button', { name: 'Sign in', exact: true }).click();
 await expect(page).toHaveURL(/\/servers$/);
 await expect(page.getByRole('heading', {name:'Proxmox installations',exact:true})).toBeVisible();
}
test('admin registers a disconnected endpoint, inspects audit and removes it', async ({page})=>{
 const errors: string[]=[];page.on('pageerror',error=>errors.push(error.message));
 await login(page,'admin');
 await expect(page.getByRole('heading',{name:'Recent audit events'})).toBeVisible();
 await page.getByRole('button',{name:'Add installation',exact:true}).click();
 await page.getByLabel('Display name').fill('Browser acceptance');
 await page.getByLabel('API URL').fill('https://192.0.2.1:8006');
 await page.getByLabel('Token ID',{exact:true}).fill('browser@pve!test');
 await page.getByLabel('Token secret',{exact:true}).fill('browser-test-secret');
 await page.getByRole('button',{name:'Register installation',exact:true}).click();
 const row=page.getByRole('row').filter({hasText:'Browser acceptance'});
 await expect(row).toContainText('disconnected');
 await expect(row).not.toContainText('browser-test-secret');
 await page.getByRole('button',{name:'Refresh audit',exact:true}).click();
 await expect(page.getByRole('cell',{name:'proxmox.server.create',exact:true}).first()).toBeVisible();
 page.once('dialog',dialog=>dialog.accept());
 await row.getByRole('button',{name:'Remove',exact:true}).click();
 await expect(row).toHaveCount(0);
 await page.getByRole('button',{name:'Sign out',exact:true}).click();
 await expect(page).toHaveURL(/\/login$/);
 await page.goto('/servers');await expect(page).toHaveURL(/\/login$/);
 expect(errors).toEqual([]);
});
test('operator can test but cannot register or remove',async({page})=>{
 await login(page,'operator');
 await expect(page.getByRole('button',{name:'Add installation',exact:true})).toHaveCount(0);
 await expect(page.getByRole('button',{name:'Remove',exact:true})).toHaveCount(0);
 await expect(page.getByRole('button',{name:'Test',exact:true}).first()).toBeVisible();
 await page.getByRole('button',{name:'Test',exact:true}).first().click();
 await expect(page.locator('.notice')).toContainText('connection');
});
test('viewer sees inventory without mutation or audit controls',async({page})=>{
 await login(page,'viewer');
 for(const name of ['Test','Remove','Add installation','Refresh audit']) await expect(page.getByRole('button',{name,exact:true})).toHaveCount(0);
});
test('failed inventory request clears loading and allows retry',async({page})=>{
 await login(page,'viewer');
 await page.route('**/api/v1/proxmox/servers',route=>route.fulfill({status:503,contentType:'application/json',body:'{"error":"unavailable"}'}));
 await page.getByRole('button',{name:'Refresh',exact:true}).click();
 await expect(page.locator('.error-banner')).toBeVisible();
 await expect(page.getByRole('button',{name:'Refresh',exact:true})).toBeEnabled();
 await page.unroute('**/api/v1/proxmox/servers');
 await page.getByRole('button',{name:'Refresh',exact:true}).click();
 await expect(page.locator('.error-banner')).toHaveCount(0);
});
