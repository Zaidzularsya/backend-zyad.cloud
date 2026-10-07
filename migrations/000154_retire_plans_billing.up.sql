-- R4-S5: pensiun plan, subscription, dan billing platform lama. Satu mesin produk/penagihan/akses
-- yang tersisa: catalog + receivable + organization_entitlements (source contract/default).

-- Entitlement internal org platform (seed lama) tidak dibutuhkan: guard memberi bypass untuk
-- org bertipe platform. Di-expire (bukan dihapus) agar riwayat audit tetap ada.
UPDATE organization_entitlements e
SET status = 'expired',
	effective_until = GREATEST(now(), e.effective_from + interval '1 microsecond'),
	reason = 'R4-S5 retired internal platform plan entitlement',
	updated_at = now()
FROM organizations o
WHERE o.id = e.organization_id
	AND o.type = 'platform'
	AND e.source = 'plan'
	AND e.status = 'active';

-- Guard: tolak berjalan bila masih ada data berbayar lama atau workspace yang aksesnya masih dari plan.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM customer_subscriptions s JOIN product_plans p ON p.id = s.plan_id
             WHERE p.plan_type IN ('paid','enterprise') AND s.status IN ('active','trialing','past_due','grace_period'))
     OR EXISTS (SELECT 1 FROM billing_invoices WHERE status IN ('draft','open'))
     OR EXISTS (SELECT 1 FROM organization_entitlements WHERE source = 'plan' AND status = 'active')
  THEN
    RAISE EXCEPTION 'R4-S5: jalankan "accessmigrate preflight" dan "accessmigrate backfill-default" dulu; data berbayar lama harus dipindah manual ke contract';
  END IF;
END $$;

DROP TABLE billing_payment_events;
DROP TABLE billing_payments;
DROP TABLE billing_invoice_items;
DROP TABLE billing_invoices;
DROP TABLE subscription_events;
DROP TABLE customer_subscriptions;
DROP TABLE product_plan_entitlements;
DROP TABLE product_plan_prices;
DROP TABLE product_plans;

-- Permission yang hilang bersama modulnya; role_permissions/user_permissions ikut terhapus (ON DELETE CASCADE).
-- organization.billing.* dan platform.product.feature.* sengaja dipertahankan.
DELETE FROM permissions
WHERE slug IN (
	'platform.billing.invoice.read',
	'platform.billing.invoice.manage',
	'platform.billing.payment.manage',
	'platform.product.plan.read',
	'platform.product.plan.manage',
	'platform.product.plan_price.read',
	'platform.product.plan_price.manage',
	'platform.product.entitlement.read',
	'platform.product.entitlement.manage',
	'platform.subscription.read',
	'platform.subscription.manage'
);
