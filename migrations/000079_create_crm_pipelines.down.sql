SELECT remove_organization_rls('crm_pipeline_stages'::regclass);
SELECT remove_organization_rls('crm_pipelines'::regclass);

DROP TABLE IF EXISTS crm_pipeline_stages;
DROP TABLE IF EXISTS crm_pipelines;
