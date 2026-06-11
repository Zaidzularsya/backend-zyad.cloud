--
-- PostgreSQL database dump
--

-- Dumped from database version 16.8 (Ubuntu 16.8-0ubuntu0.24.04.1)
-- Dumped by pg_dump version 16.8 (Ubuntu 16.8-0ubuntu0.24.04.1)

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: public; Type: SCHEMA; Schema: -; Owner: -
--

CREATE SCHEMA IF NOT EXISTS public;


--
-- Name: license_type; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.license_type AS ENUM (
    'none',
    'single',
    'multi',
    'subscription'
);


--
-- Name: price_type; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.price_type AS ENUM (
    'base',
    'cost',
    'promo',
    'tiered',
    'volume',
    'one_time',
    'discount',
    'reseller',
    'seasonal'
);


--
-- Name: product_metadata_value_type; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.product_metadata_value_type AS ENUM (
    'string',
    'number',
    'date'
);


--
-- Name: product_type; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.product_type AS ENUM (
    'physical',
    'service',
    'subscription',
    'digital',
    'bundle'
);


--
-- Name: relation_type; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.relation_type AS ENUM (
    'include',
    'optional',
    'addon',
    'requires',
    'mutually-exclusive'
);


--
-- Name: resource_kind; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.resource_kind AS ENUM (
    'SERVICE',
    'ACCESS',
    'DEVICE',
    'INFRA',
    'ACCOUNT'
);


--
-- Name: service_type; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.service_type AS ENUM (
    'Monthly',
    'Quarterly',
    'Semi Annualy',
    'Annualy'
);


--
-- Name: transaction_status; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.transaction_status AS ENUM (
    'pending',
    'success',
    'failed'
);


--
-- Name: tg_set_updated_at(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.tg_set_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  NEW.updated_at := now();
  RETURN NEW;
END$$;


--
-- Name: touch_orders_updated_at(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.touch_orders_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  NEW.updated_at := now();
  RETURN NEW;
END; $$;


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: ORDER_DETAIL_TYPES; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public."ORDER_DETAIL_TYPES" (
    "ORDER_DETAIL_TYPE_ID" integer NOT NULL,
    "NAME" character varying(32) DEFAULT NULL::character varying,
    "DESCRIPTION" character varying(64) DEFAULT NULL::character varying
);


--
-- Name: ORDER_DETAIL_TYPES_ORDER_DETAIL_TYPE_ID_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public."ORDER_DETAIL_TYPES_ORDER_DETAIL_TYPE_ID_seq"
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: ORDER_DETAIL_TYPES_ORDER_DETAIL_TYPE_ID_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public."ORDER_DETAIL_TYPES_ORDER_DETAIL_TYPE_ID_seq" OWNED BY public."ORDER_DETAIL_TYPES"."ORDER_DETAIL_TYPE_ID";


--
-- Name: account; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.account (
    account_id uuid DEFAULT gen_random_uuid() NOT NULL,
    account_type_id integer NOT NULL,
    nik character varying(16),
    passport_number character varying(20),
    full_name character varying(100) NOT NULL,
    birth_place character varying(100),
    birth_date date,
    gender character(1),
    blood_type character(2),
    address text,
    rt_rw character varying(10),
    village character varying(100),
    district character varying(100),
    religion character varying(50),
    marital_status character varying(20),
    occupation character varying(100),
    nationality character varying(50) DEFAULT 'INDONESIA'::character varying NOT NULL,
    issued_date date DEFAULT CURRENT_DATE NOT NULL,
    expiry_date date,
    photo_url text,
    signature_url text,
    is_active boolean DEFAULT true NOT NULL,
    is_deleted boolean DEFAULT false NOT NULL,
    is_verified boolean DEFAULT false NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    org_id uuid DEFAULT '00000000-0000-0000-0000-000000000000'::uuid NOT NULL,
    parent_account_id uuid,
    type text DEFAULT 'CUSTOMER_ACCOUNT'::text NOT NULL,
    deleted_at timestamp without time zone,
    CONSTRAINT account_blood_type_check CHECK ((blood_type = ANY (ARRAY['A'::bpchar, 'B'::bpchar, 'AB'::bpchar, 'O'::bpchar]))),
    CONSTRAINT account_gender_check CHECK ((gender = ANY (ARRAY['M'::bpchar, 'F'::bpchar]))),
    CONSTRAINT account_marital_status_check CHECK (((marital_status)::text = ANY (ARRAY[('Belum Kawin'::character varying)::text, ('Kawin'::character varying)::text, ('Cerai Hidup'::character varying)::text, ('Cerai Mati'::character varying)::text]))),
    CONSTRAINT account_type_check CHECK ((type = ANY (ARRAY['CUSTOMER_ACCOUNT'::text, 'BILLING_ACCOUNT'::text, 'SERVICE_ACCOUNT'::text])))
);


--
-- Name: account_contact_types; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.account_contact_types (
    account_contact_type_id integer NOT NULL,
    name character varying(50) NOT NULL,
    description text,
    is_active boolean DEFAULT true NOT NULL,
    is_deleted boolean DEFAULT false NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: account_contact_types_account_contact_type_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.account_contact_types_account_contact_type_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: account_contact_types_account_contact_type_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.account_contact_types_account_contact_type_id_seq OWNED BY public.account_contact_types.account_contact_type_id;


--
-- Name: account_contacts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.account_contacts (
    account_contact_id integer NOT NULL,
    account_id uuid NOT NULL,
    account_contact_type_id integer NOT NULL,
    contact_value character varying(255) NOT NULL,
    reference text,
    is_primary boolean DEFAULT false NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    is_deleted boolean DEFAULT false NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: account_contacts_account_contact_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.account_contacts_account_contact_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: account_contacts_account_contact_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.account_contacts_account_contact_id_seq OWNED BY public.account_contacts.account_contact_id;


--
-- Name: account_types; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.account_types (
    account_type_id integer NOT NULL,
    name character varying(50) NOT NULL,
    description text,
    is_active boolean DEFAULT true NOT NULL,
    is_deleted boolean DEFAULT false NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: account_types_account_type_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.account_types_account_type_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: account_types_account_type_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.account_types_account_type_id_seq OWNED BY public.account_types.account_type_id;


--
-- Name: asset_detail_types; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.asset_detail_types (
    id integer NOT NULL,
    name character varying(32) DEFAULT NULL::character varying,
    description character varying(64) DEFAULT NULL::character varying
);


--
-- Name: asset_detail_types_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.asset_detail_types_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: asset_detail_types_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.asset_detail_types_id_seq OWNED BY public.asset_detail_types.id;


--
-- Name: asset_details; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.asset_details (
    asset_detail_id bigint NOT NULL,
    asset_id bigint,
    asset_detail_type_id integer,
    update_dtm timestamp with time zone,
    xn1 integer,
    xn2 integer,
    xn3 integer,
    xn4 integer,
    xn5 integer,
    xn6 bigint,
    xn7 integer,
    xn8 bigint,
    xs1 character varying(64),
    xs2 character varying(64),
    xs3 character varying(512),
    xs4 character varying(64),
    xs5 character varying(1000),
    xs6 character varying(1000),
    xs7 character varying(128),
    xs8 character varying(256),
    xs9 character varying(512),
    xs10 character varying(512),
    xs11 character varying(2000),
    xs12 character varying(512),
    xs13 character varying(1000),
    xs14 character varying(1000),
    xs15 character varying(1000),
    xs16 character varying(1000),
    xs17 character varying(1000),
    xs18 character varying(1000),
    xs19 character varying(512),
    xs20 character varying(512),
    xs21 character varying(640),
    xs22 character varying(640),
    xs23 character varying(1000),
    xs24 character varying(1000),
    xs25 character varying(512),
    xs26 character varying(512),
    xs27 character varying(512),
    xs28 character varying(512),
    xs29 character varying(512),
    xs30 character varying(640),
    xs31 character varying(640),
    xs32 character varying(640),
    xs33 text,
    xs34 character varying(4000),
    xs35 character varying(4000),
    xs36 character varying(4000),
    xs37 text,
    xs38 character varying(640),
    xs39 character varying(1000),
    xs40 text,
    xd1 timestamp without time zone,
    xd2 timestamp without time zone,
    xd3 timestamp without time zone
);


--
-- Name: asset_details_asset_detail_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.asset_details_asset_detail_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: asset_details_asset_detail_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.asset_details_asset_detail_id_seq OWNED BY public.asset_details.asset_detail_id;


--
-- Name: asset_statuses; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.asset_statuses (
    asset_status_id integer NOT NULL,
    name character varying(128) DEFAULT NULL::character varying,
    description character varying(512) DEFAULT NULL::character varying
);


--
-- Name: asset_statuses_asset_status_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.asset_statuses_asset_status_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: asset_statuses_asset_status_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.asset_statuses_asset_status_id_seq OWNED BY public.asset_statuses.asset_status_id;


--
-- Name: asset_subtypes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.asset_subtypes (
    asset_subtype_id integer NOT NULL,
    name character varying(64) DEFAULT NULL::character varying,
    description character varying(128) DEFAULT NULL::character varying,
    asset_type_id integer
);


--
-- Name: asset_subtypes_asset_subtype_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.asset_subtypes_asset_subtype_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: asset_subtypes_asset_subtype_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.asset_subtypes_asset_subtype_id_seq OWNED BY public.asset_subtypes.asset_subtype_id;


--
-- Name: asset_types; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.asset_types (
    asset_type_id integer NOT NULL,
    name character varying(32) DEFAULT NULL::character varying,
    description character varying(64) DEFAULT NULL::character varying
);


--
-- Name: asset_types_asset_type_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.asset_types_asset_type_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: asset_types_asset_type_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.asset_types_asset_type_id_seq OWNED BY public.asset_types.asset_type_id;


--
-- Name: assets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.assets (
    asset_id bigint NOT NULL,
    asset_code character varying(16),
    asset_type_id integer,
    asset_subtype_id integer,
    asset_status_id integer,
    asset_desc character varying(64),
    customer_desc character varying(64),
    product_desc character varying(64),
    create_dtm timestamp without time zone,
    close_dtm timestamp without time zone,
    xn1 integer,
    xn2 integer,
    xn3 integer,
    xn4 integer,
    xn5 integer,
    xs1 character varying(32),
    xs2 character varying(64),
    xs3 character varying(128),
    xs4 character varying(256),
    xs5 character varying(512),
    xs6 character varying(512),
    xs7 character varying(512),
    xs8 character varying(512),
    xs9 character varying(512),
    xs10 character varying(512),
    xs11 character varying(512),
    xs12 character varying(512),
    xs13 character varying(512),
    xs14 character varying(512),
    xs15 character varying(1000),
    xs16 character varying(1000),
    xs17 character varying(1000),
    xs18 character varying(1000),
    xs19 character varying(1000),
    xs20 character varying(4000),
    xd1 timestamp without time zone,
    xd2 timestamp without time zone,
    xd3 timestamp without time zone,
    create_user_id uuid
);


--
-- Name: assets_asset_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.assets_asset_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: assets_asset_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.assets_asset_id_seq OWNED BY public.assets.asset_id;


--
-- Name: billing_detail_types; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.billing_detail_types (
    id integer NOT NULL,
    name character varying(32) DEFAULT NULL::character varying,
    description character varying(64) DEFAULT NULL::character varying
);


--
-- Name: billing_detail_types_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.billing_detail_types_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: billing_detail_types_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.billing_detail_types_id_seq OWNED BY public.billing_detail_types.id;


--
-- Name: billing_details; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.billing_details (
    billing_detail_id bigint NOT NULL,
    billing_id bigint,
    billing_detail_type_id integer,
    update_dtm timestamp without time zone,
    xn1 integer,
    xn2 integer,
    xn3 integer,
    xn4 integer,
    xn5 integer,
    xn6 bigint,
    xn7 integer,
    xn8 bigint,
    xs1 character varying(64),
    xs2 character varying(64),
    xs3 character varying(512),
    xs4 character varying(64),
    xs5 character varying(1000),
    xs6 character varying(1000),
    xs7 character varying(128),
    xs8 character varying(256),
    xs9 character varying(512),
    xs10 character varying(512),
    xs11 character varying(2000),
    xs12 character varying(512),
    xs13 character varying(1000),
    xs14 character varying(1000),
    xs15 character varying(1000),
    xs16 character varying(1000),
    xs17 character varying(1000),
    xs18 character varying(1000),
    xs19 character varying(512),
    xs20 character varying(512),
    xs21 character varying(640),
    xs22 character varying(640),
    xs23 character varying(1000),
    xs24 character varying(1000),
    xs25 character varying(512),
    xs26 character varying(512),
    xs27 character varying(512),
    xs28 character varying(512),
    xs29 character varying(512),
    xs30 character varying(640),
    xs31 character varying(640),
    xs32 character varying(640),
    xs33 text,
    xs34 character varying(4000),
    xs35 character varying(4000),
    xs36 character varying(4000),
    xs37 text,
    xs38 character varying(640),
    xs39 character varying(1000),
    xs40 text,
    xd1 timestamp without time zone,
    xd2 timestamp without time zone,
    xd3 timestamp without time zone
);


--
-- Name: billing_details_billing_detail_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.billing_details_billing_detail_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: billing_details_billing_detail_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.billing_details_billing_detail_id_seq OWNED BY public.billing_details.billing_detail_id;


--
-- Name: billing_invoice_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.billing_invoice_items (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    invoice_id uuid NOT NULL,
    line_no integer NOT NULL,
    description character varying(255) NOT NULL,
    qty numeric(18,4) DEFAULT 1 NOT NULL,
    unit_price numeric(18,2) DEFAULT 0 NOT NULL,
    amount numeric(18,2) DEFAULT 0 NOT NULL,
    revenue_account_code character varying(20),
    meta jsonb DEFAULT '{}'::jsonb NOT NULL
);


--
-- Name: billing_invoices; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.billing_invoices (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    account_id uuid,
    resource_instance_id uuid,
    billing_no character varying(50) NOT NULL,
    billing_period_from date,
    billing_period_to date,
    issue_date date DEFAULT CURRENT_DATE NOT NULL,
    due_date date,
    currency character(3) DEFAULT 'IDR'::bpchar NOT NULL,
    subtotal numeric(18,2) DEFAULT 0 NOT NULL,
    tax_amount numeric(18,2) DEFAULT 0 NOT NULL,
    total_amount numeric(18,2) DEFAULT 0 NOT NULL,
    status character varying(24) DEFAULT 'DRAFT'::character varying NOT NULL,
    meta jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);


--
-- Name: billing_payment_applications; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.billing_payment_applications (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    payment_id uuid NOT NULL,
    invoice_id uuid NOT NULL,
    org_id uuid NOT NULL,
    account_id uuid NOT NULL,
    applied_amount numeric(18,2) NOT NULL,
    applied_at timestamp with time zone DEFAULT now(),
    meta jsonb DEFAULT '{}'::jsonb NOT NULL
);


--
-- Name: billing_payments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.billing_payments (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    account_id uuid NOT NULL,
    payment_no character varying(50) NOT NULL,
    payment_date date DEFAULT CURRENT_DATE NOT NULL,
    method character varying(24) NOT NULL,
    reference_no character varying(50),
    currency character(3) DEFAULT 'IDR'::bpchar NOT NULL,
    amount_received numeric(18,2) NOT NULL,
    unapplied_amount numeric(18,2) DEFAULT 0 NOT NULL,
    status character varying(24) DEFAULT 'RECEIVED'::character varying NOT NULL,
    meta jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);


--
-- Name: billing_statuses; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.billing_statuses (
    billing_status_id integer NOT NULL,
    name character varying(128) DEFAULT NULL::character varying,
    description character varying(512) DEFAULT NULL::character varying
);


--
-- Name: billing_statuses_billing_status_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.billing_statuses_billing_status_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: billing_statuses_billing_status_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.billing_statuses_billing_status_id_seq OWNED BY public.billing_statuses.billing_status_id;


--
-- Name: billing_subtypes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.billing_subtypes (
    billing_subtype_id integer NOT NULL,
    name character varying(64) DEFAULT NULL::character varying,
    description character varying(128) DEFAULT NULL::character varying,
    billing_type_id integer
);


--
-- Name: billing_subtypes_billing_subtype_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.billing_subtypes_billing_subtype_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: billing_subtypes_billing_subtype_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.billing_subtypes_billing_subtype_id_seq OWNED BY public.billing_subtypes.billing_subtype_id;


--
-- Name: billing_types; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.billing_types (
    billing_type_id integer NOT NULL,
    name character varying(32) DEFAULT NULL::character varying,
    description character varying(64) DEFAULT NULL::character varying
);


--
-- Name: billing_types_billing_type_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.billing_types_billing_type_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: billing_types_billing_type_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.billing_types_billing_type_id_seq OWNED BY public.billing_types.billing_type_id;


--
-- Name: billings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.billings (
    billing_id bigint NOT NULL,
    billing_code character varying(16),
    billing_type_id integer,
    billing_subtype_id integer,
    billing_status_id integer,
    billing_desc character varying(64),
    customer_desc character varying(64),
    product_desc character varying(64),
    create_dtm timestamp without time zone,
    close_dtm timestamp without time zone,
    xn1 integer,
    xn2 integer,
    xn3 integer,
    xn4 integer,
    xn5 integer,
    xs1 character varying(32),
    xs2 character varying(64),
    xs3 character varying(128),
    xs4 character varying(256),
    xs5 character varying(512),
    xs6 character varying(512),
    xs7 character varying(512),
    xs8 character varying(512),
    xs9 character varying(512),
    xs10 character varying(512),
    xs11 character varying(512),
    xs12 character varying(512),
    xs13 character varying(512),
    xs14 character varying(512),
    xs15 character varying(1000),
    xs16 character varying(1000),
    xs17 character varying(1000),
    xs18 character varying(1000),
    xs19 character varying(1000),
    xs20 character varying(4000),
    xd1 timestamp without time zone,
    xd2 timestamp without time zone,
    xd3 timestamp without time zone,
    create_user_id uuid,
    org_id uuid,
    account_id uuid,
    contract_id uuid,
    total_amount numeric(18,2),
    tax_amount numeric(18,2),
    currency character varying(3) DEFAULT 'IDR'::character varying
);


--
-- Name: billings_billing_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.billings_billing_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: billings_billing_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.billings_billing_id_seq OWNED BY public.billings.billing_id;


--
-- Name: brands_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.brands_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    MAXVALUE 2147483647
    CACHE 1;


--
-- Name: brands; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.brands (
    id integer DEFAULT nextval('public.brands_id_seq'::regclass) NOT NULL,
    name character varying(255),
    description text,
    created_at timestamp without time zone,
    updated_at timestamp without time zone
);


--
-- Name: bundle_components; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.bundle_components (
    bundle_offering_id uuid NOT NULL,
    component_offering_id uuid NOT NULL,
    quantity integer DEFAULT 1 NOT NULL
);


--
-- Name: categories_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.categories_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    MAXVALUE 2147483647
    CACHE 1;


--
-- Name: category_attribute_schemas; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.category_attribute_schemas (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    category_id uuid NOT NULL,
    attribute_key character varying(100) NOT NULL,
    label character varying(255),
    data_type character varying(50) NOT NULL,
    is_required boolean DEFAULT false,
    allowed_values jsonb,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    CONSTRAINT category_attribute_schemas_data_type_check CHECK (((data_type)::text = ANY (ARRAY[('STRING'::character varying)::text, ('INTEGER'::character varying)::text, ('BOOLEAN'::character varying)::text, ('ENUM'::character varying)::text, ('DECIMAL'::character varying)::text])))
);


--
-- Name: contact_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.contact_requests (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name character varying(150) NOT NULL,
    email character varying(150) NOT NULL,
    phone character varying(30),
    intent character varying(30) NOT NULL,
    message text NOT NULL,
    source character varying(50),
    plan_interest character varying(50),
    priority character varying(20) DEFAULT 'NORMAL'::character varying,
    ip_address inet,
    user_agent text,
    status character varying(20) DEFAULT 'NEW'::character varying,
    assigned_to uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT contact_requests_intent_check CHECK (((intent)::text = ANY ((ARRAY['SALES'::character varying, 'SUPPORT'::character varying, 'PARTNERSHIP'::character varying, 'GENERAL'::character varying])::text[]))),
    CONSTRAINT contact_requests_priority_check CHECK (((priority)::text = ANY ((ARRAY['LOW'::character varying, 'NORMAL'::character varying, 'HIGH'::character varying])::text[]))),
    CONSTRAINT contact_requests_status_check CHECK (((status)::text = ANY ((ARRAY['NEW'::character varying, 'ASSIGNED'::character varying, 'IN_PROGRESS'::character varying, 'CLOSED'::character varying])::text[])))
);


--
-- Name: contract_lifecycle_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.contract_lifecycle_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    contract_id uuid NOT NULL,
    org_id uuid NOT NULL,
    event_type character varying(50) NOT NULL,
    previous_state character varying(24),
    new_state character varying(24) NOT NULL,
    reason text,
    ref_invoice_id uuid,
    ref_user_id uuid,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: contracts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.contracts (
    contract_id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    account_id uuid NOT NULL,
    product_id uuid,
    contract_code character varying(32) NOT NULL,
    contract_desc character varying(128),
    start_date date NOT NULL,
    end_date date,
    status character varying(20) NOT NULL,
    billing_cycle character varying(16) NOT NULL,
    billing_day integer,
    next_billing_date date,
    amount_before_tax numeric(18,2) NOT NULL,
    tax_amount numeric(18,2) DEFAULT 0 NOT NULL,
    total_amount numeric(18,2) NOT NULL,
    currency character varying(3) DEFAULT 'IDR'::character varying NOT NULL,
    auto_renew boolean DEFAULT true NOT NULL,
    is_template boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: deposit_accounts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.deposit_accounts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    balance numeric(18,2) DEFAULT 0 NOT NULL,
    currency character varying(10) DEFAULT 'IDR'::character varying,
    status character varying(20) DEFAULT 'active'::character varying NOT NULL,
    created_at timestamp without time zone DEFAULT now(),
    updated_at timestamp without time zone DEFAULT now()
);


--
-- Name: deposit_topup_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.deposit_topup_requests (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    deposit_account_id uuid NOT NULL,
    amount numeric(18,2) NOT NULL,
    method character varying(50),
    status character varying(20) DEFAULT 'pending'::character varying NOT NULL,
    proof_url text,
    created_at timestamp without time zone DEFAULT now(),
    verified_at timestamp without time zone
);


--
-- Name: deposit_transactions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.deposit_transactions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    deposit_account_id uuid NOT NULL,
    type character varying(20) NOT NULL,
    amount numeric(18,2) NOT NULL,
    balance_before numeric(18,2) NOT NULL,
    balance_after numeric(18,2) NOT NULL,
    reference_id character varying(100),
    description text,
    status character varying(20) DEFAULT 'success'::character varying NOT NULL,
    method character varying(50),
    created_at timestamp without time zone DEFAULT now(),
    updated_at timestamp without time zone DEFAULT now()
);


--
-- Name: deposit_withdraw_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.deposit_withdraw_requests (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    deposit_account_id uuid NOT NULL,
    amount numeric(18,2) NOT NULL,
    destination text NOT NULL,
    method character varying(50),
    status character varying(20) DEFAULT 'pending'::character varying NOT NULL,
    approved_by uuid,
    note text,
    paid_at timestamp without time zone,
    created_at timestamp without time zone DEFAULT now(),
    updated_at timestamp without time zone DEFAULT now()
);


--
-- Name: digital_files; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.digital_files (
    id integer NOT NULL,
    product_id integer,
    file_path character varying(255),
    file_size numeric(10,2),
    file_format character varying(50),
    download_limit integer,
    created_at timestamp without time zone
);


--
-- Name: digital_files_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.digital_files_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    MAXVALUE 2147483647
    CACHE 1;


--
-- Name: gl_accounts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gl_accounts (
    account_code character varying(30) NOT NULL,
    name character varying(100) NOT NULL,
    account_type character varying(20) NOT NULL,
    org_id uuid NOT NULL,
    is_active boolean DEFAULT true,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    id uuid DEFAULT gen_random_uuid() NOT NULL
);


--
-- Name: gl_journal_entries; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gl_journal_entries (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    journal_code character varying(30),
    journal_date date DEFAULT CURRENT_DATE NOT NULL,
    source_type character varying(30) NOT NULL,
    source_id uuid NOT NULL,
    source_table character varying(30) NOT NULL,
    description character varying(256),
    org_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    created_by uuid
);


--
-- Name: gl_journal_lines; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.gl_journal_lines (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    journal_id uuid NOT NULL,
    line_no integer NOT NULL,
    account_code character varying(30) NOT NULL,
    debit numeric(18,2) DEFAULT 0,
    credit numeric(18,2) DEFAULT 0,
    org_id uuid NOT NULL
);


--
-- Name: landing_pages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.landing_pages (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    owner_type character varying(32) NOT NULL,
    owner_id uuid,
    code character varying(64) NOT NULL,
    title character varying(255) NOT NULL,
    description text,
    primary_domain character varying(255),
    is_custom_domain boolean DEFAULT false NOT NULL,
    status character varying(24) DEFAULT 'DRAFT'::character varying NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    config jsonb DEFAULT '{}'::jsonb NOT NULL,
    seo jsonb DEFAULT '{}'::jsonb NOT NULL,
    cta jsonb DEFAULT '{}'::jsonb NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    published_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    created_by uuid,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_by uuid,
    CONSTRAINT landing_pages_owner_type_chk CHECK (((owner_type)::text = ANY ((ARRAY['PLATFORM'::character varying, 'ACCOUNT'::character varying, 'RESOURCE_INSTANCE'::character varying])::text[]))),
    CONSTRAINT landing_pages_status_chk CHECK (((status)::text = ANY ((ARRAY['DRAFT'::character varying, 'PUBLISHED'::character varying, 'SUSPENDED'::character varying, 'ARCHIVED'::character varying])::text[])))
);


--
-- Name: menus; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.menus (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    module_id uuid NOT NULL,
    label character varying NOT NULL,
    route character varying NOT NULL,
    icon character varying,
    parent_id uuid,
    sort_order integer DEFAULT 0,
    created_at timestamp without time zone DEFAULT now(),
    menu_group character varying(150),
    is_active boolean DEFAULT true
);


--
-- Name: modules; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.modules (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    module_key character varying NOT NULL,
    label character varying NOT NULL,
    description text,
    icon character varying,
    sort_order integer DEFAULT 0,
    created_at timestamp without time zone DEFAULT now(),
    updated_at timestamp without time zone DEFAULT now()
);


--
-- Name: nas; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.nas (
    id integer NOT NULL,
    nasname text NOT NULL,
    shortname text NOT NULL,
    type text DEFAULT 'other'::text NOT NULL,
    ports integer,
    secret text NOT NULL,
    server text,
    community text,
    description text
);


--
-- Name: nas_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.nas_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: nas_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.nas_id_seq OWNED BY public.nas.id;


--
-- Name: nasreload; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.nasreload (
    nasipaddress inet NOT NULL,
    reloadtime timestamp with time zone NOT NULL
);


--
-- Name: offering_publications; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.offering_publications (
    offering_id uuid NOT NULL,
    visible_to_org_id uuid NOT NULL,
    status text DEFAULT 'ENABLED'::text NOT NULL,
    price_override jsonb,
    effective_from timestamp with time zone,
    effective_to timestamp with time zone,
    channel text,
    note text,
    CONSTRAINT offering_publications_status_check CHECK ((status = ANY (ARRAY['ENABLED'::text, 'DISABLED'::text])))
);


--
-- Name: offering_resource_templates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.offering_resource_templates (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    offering_id uuid NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    name character varying(64) NOT NULL,
    description character varying(255),
    form_schema jsonb NOT NULL,
    ui_schema jsonb NOT NULL,
    monitor_schema jsonb NOT NULL,
    provision_map jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);


--
-- Name: order_activities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.order_activities (
    order_activity_id bigint NOT NULL,
    order_id bigint,
    update_dtm timestamp without time zone,
    xn1 bigint,
    xn2 integer,
    xn3 integer,
    xn4 integer,
    xn5 integer,
    xn6 integer,
    xn7 integer,
    xn8 integer,
    xn9 integer,
    xn10 integer,
    xs1 character varying(128),
    xs2 character varying(128),
    xs3 character varying(1000),
    xs4 character varying(128),
    xs5 character varying(4000),
    xs6 character varying(4000),
    xs7 character varying(4000),
    xs8 character varying(4000),
    xs9 character varying(4000),
    xs10 character varying(4000),
    xd1 timestamp without time zone,
    xd2 timestamp without time zone,
    attr jsonb,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);


--
-- Name: order_activities_order_activity_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.order_activities_order_activity_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: order_activities_order_activity_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.order_activities_order_activity_id_seq OWNED BY public.order_activities.order_activity_id;


--
-- Name: order_activities_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.order_activities_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: order_activities_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.order_activities_seq OWNED BY public.order_activities.order_activity_id;


--
-- Name: order_detail_types; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.order_detail_types (
    id integer NOT NULL,
    name character varying(32) DEFAULT NULL::character varying,
    description character varying(64) DEFAULT NULL::character varying
);


--
-- Name: order_detail_types_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.order_detail_types_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: order_detail_types_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.order_detail_types_id_seq OWNED BY public.order_detail_types.id;


--
-- Name: order_details; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.order_details (
    order_detail_id bigint NOT NULL,
    order_id bigint,
    type integer,
    update_dtm timestamp without time zone,
    xn1 integer,
    xn2 integer,
    xn3 integer,
    xn4 integer,
    xn5 integer,
    xn6 bigint,
    xn7 integer,
    xn8 bigint,
    xs1 character varying(64),
    xs2 character varying(64),
    xs3 character varying(512),
    xs4 character varying(64),
    xs5 character varying(1000),
    xs6 character varying(1000),
    xs7 character varying(128),
    xs8 character varying(256),
    xs9 character varying(512),
    xs10 character varying(512),
    xs11 character varying(2000),
    xs12 character varying(512),
    xs13 character varying(1000),
    xs14 character varying(1000),
    xs15 character varying(1000),
    xs16 character varying(1000),
    xs17 character varying(1000),
    xs18 character varying(1000),
    xs19 character varying(512),
    xs20 character varying(512),
    xs21 character varying(640),
    xs22 character varying(640),
    xs23 character varying(1000),
    xs24 character varying(1000),
    xs25 character varying(512),
    xs26 character varying(512),
    xs27 character varying(512),
    xs28 character varying(512),
    xs29 character varying(512),
    xs30 character varying(640),
    xs31 character varying(640),
    xs32 character varying(640),
    xs33 text,
    xs34 character varying(4000),
    xs35 character varying(4000),
    xs36 character varying(4000),
    xs37 text,
    xs38 character varying(640),
    xs39 character varying(1000),
    xs40 text,
    xd1 timestamp without time zone,
    xd2 timestamp without time zone,
    xd3 timestamp without time zone,
    attr jsonb,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);


--
-- Name: order_details_order_detail_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.order_details_order_detail_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: order_details_order_detail_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.order_details_order_detail_id_seq OWNED BY public.order_details.order_detail_id;


--
-- Name: order_status_transitions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.order_status_transitions (
    from_status_id integer NOT NULL,
    to_status_id integer NOT NULL,
    role_required character varying(64),
    is_default boolean DEFAULT false
);


--
-- Name: order_statuses; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.order_statuses (
    order_status_id integer NOT NULL,
    name character varying(128) DEFAULT NULL::character varying,
    description character varying(512) DEFAULT NULL::character varying
);


--
-- Name: order_statuses_order_status_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.order_statuses_order_status_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: order_statuses_order_status_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.order_statuses_order_status_id_seq OWNED BY public.order_statuses.order_status_id;


--
-- Name: order_subtypes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.order_subtypes (
    order_subtype_id integer NOT NULL,
    name character varying(64) DEFAULT NULL::character varying,
    description character varying(128) DEFAULT NULL::character varying,
    order_type_id integer
);


--
-- Name: order_subtypes_order_subtype_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.order_subtypes_order_subtype_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: order_subtypes_order_subtype_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.order_subtypes_order_subtype_id_seq OWNED BY public.order_subtypes.order_subtype_id;


--
-- Name: order_types; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.order_types (
    order_type_id integer NOT NULL,
    name character varying(32) DEFAULT NULL::character varying,
    description character varying(64) DEFAULT NULL::character varying
);


--
-- Name: order_types_order_type_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.order_types_order_type_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: order_types_order_type_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.order_types_order_type_id_seq OWNED BY public.order_types.order_type_id;


--
-- Name: orders; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.orders (
    order_id bigint NOT NULL,
    order_code character varying(16),
    order_type_id integer,
    order_subtype_id integer,
    order_status_id integer,
    order_desc character varying(64),
    customer_desc character varying(64),
    product_desc character varying(64),
    create_dtm timestamp without time zone,
    close_dtm timestamp without time zone,
    xn1 integer,
    xn2 integer,
    xn3 integer,
    xn4 integer,
    xn5 integer,
    xs1 character varying(32),
    xs2 character varying(64),
    xs3 character varying(128),
    xs4 character varying(256),
    xs5 character varying(512),
    xs6 character varying(512),
    xs7 character varying(512),
    xs8 character varying(512),
    xs9 character varying(512),
    xs10 character varying(512),
    xs11 character varying(512),
    xs12 character varying(512),
    xs13 character varying(512),
    xs14 character varying(512),
    xs15 character varying(1000),
    xs16 character varying(1000),
    xs17 character varying(1000),
    xs18 character varying(1000),
    xs19 character varying(1000),
    xs20 character varying(4000),
    xd1 timestamp without time zone,
    xd2 timestamp without time zone,
    xd3 timestamp without time zone,
    create_user_id uuid,
    attr jsonb,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    updated_user_id uuid,
    org_id uuid,
    buyer_org_id uuid,
    buyer_account_id uuid,
    seller_org_id uuid,
    currency character varying(8) DEFAULT 'IDR'::character varying NOT NULL,
    total_amount numeric(18,2) DEFAULT 0 NOT NULL,
    status character varying(24)
);


--
-- Name: orders_order_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.orders_order_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: orders_order_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.orders_order_id_seq OWNED BY public.orders.order_id;


--
-- Name: orgs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.orgs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    parent_org_id uuid
);


--
-- Name: payment_applications; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.payment_applications (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    payment_id uuid NOT NULL,
    invoice_id uuid NOT NULL,
    applied_amount numeric(18,2) NOT NULL,
    created_at timestamp with time zone DEFAULT now()
);


--
-- Name: payments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.payments (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    org_id uuid NOT NULL,
    account_id uuid,
    payment_no character varying(50) NOT NULL,
    payment_date date DEFAULT CURRENT_DATE NOT NULL,
    amount numeric(18,2) NOT NULL,
    currency character(3) DEFAULT 'IDR'::bpchar NOT NULL,
    method character varying(50),
    reference_no character varying(100),
    status character varying(24) DEFAULT 'RECORDED'::character varying NOT NULL,
    meta jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);


--
-- Name: permissions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.permissions (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    permission_name character varying,
    description text,
    created_at timestamp without time zone,
    updated_at timestamp without time zone,
    module_id uuid
);


--
-- Name: playing_with_neon; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.playing_with_neon (
    id integer NOT NULL,
    name text,
    value real
);


--
-- Name: playing_with_neon_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.playing_with_neon_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    MAXVALUE 2147483647
    CACHE 1;


--
-- Name: pricing_models; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.pricing_models (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    offering_id uuid NOT NULL,
    type character varying(50) NOT NULL,
    currency character varying(3) DEFAULT 'IDR'::character varying NOT NULL,
    amount numeric(15,2) NOT NULL,
    billing_cycle character varying(20),
    conditions jsonb,
    created_at timestamp with time zone DEFAULT now(),
    description character varying(255),
    CONSTRAINT pricing_models_billing_cycle_check CHECK (((billing_cycle)::text = ANY (ARRAY[('DAILY'::character varying)::text, ('WEEKLY'::character varying)::text, ('MONTHLY'::character varying)::text, ('QUARTERLY'::character varying)::text, ('YEARLY'::character varying)::text]))),
    CONSTRAINT pricing_models_type_check CHECK (((type)::text = ANY (ARRAY[('ONE_TIME'::character varying)::text, ('RECURRING'::character varying)::text, ('USAGE_BASED'::character varying)::text])))
);


--
-- Name: product_attributes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.product_attributes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    product_id integer NOT NULL,
    key text NOT NULL,
    value text NOT NULL,
    value_type text DEFAULT 'string'::text
);


--
-- Name: product_categories; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.product_categories (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name character varying(100) NOT NULL,
    description text,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);


--
-- Name: product_configurations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.product_configurations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    product_id integer NOT NULL,
    key text NOT NULL,
    label text,
    input_type text,
    options jsonb,
    default_value text
);


--
-- Name: product_images; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.product_images (
    id integer NOT NULL,
    product_id integer,
    image_path character varying(255),
    is_primary boolean,
    created_at timestamp without time zone
);


--
-- Name: product_images_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.product_images_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    MAXVALUE 2147483647
    CACHE 1;


--
-- Name: product_metadata_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.product_metadata_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    MAXVALUE 2147483647
    CACHE 1;


--
-- Name: product_offering_attributes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.product_offering_attributes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    offering_id uuid NOT NULL,
    spec_characteristic_id uuid,
    name character varying(100) NOT NULL,
    data_type character varying(20) NOT NULL,
    value_text text,
    value_number numeric,
    value_boolean boolean,
    value_json jsonb,
    allowed_values jsonb,
    unit character varying(20),
    is_required boolean DEFAULT false,
    is_configurable boolean DEFAULT false,
    scope character varying(20) DEFAULT 'commercial'::character varying,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    CONSTRAINT product_offering_attributes_data_type_check CHECK (((data_type)::text = ANY (ARRAY[('string'::character varying)::text, ('number'::character varying)::text, ('boolean'::character varying)::text, ('enum'::character varying)::text, ('json'::character varying)::text]))),
    CONSTRAINT product_offering_attributes_scope_check CHECK (((scope)::text = ANY (ARRAY[('commercial'::character varying)::text, ('technical'::character varying)::text])))
);


--
-- Name: product_offerings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.product_offerings (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    spec_id uuid NOT NULL,
    name character varying(255) NOT NULL,
    description text,
    status character varying(50) DEFAULT 'DRAFT'::character varying,
    valid_from timestamp with time zone,
    valid_to timestamp with time zone,
    is_bundle boolean DEFAULT false,
    metadata jsonb,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    owner_org_id uuid,
    publish_scope text DEFAULT 'PRIVATE'::text NOT NULL,
    visibility_meta jsonb DEFAULT '{"region": "*", "channel": "*", "denied_orgs": [], "allowed_orgs": ["*"], "allowed_roles": ["*"], "visibility_scope": "PRIVATE"}'::jsonb NOT NULL,
    CONSTRAINT product_offerings_publish_scope_check CHECK ((publish_scope = ANY (ARRAY['PRIVATE'::text, 'ORG'::text, 'PUBLIC'::text, 'PARTNER'::text]))),
    CONSTRAINT product_offerings_status_check CHECK (((status)::text = ANY (ARRAY[('DRAFT'::character varying)::text, ('ACTIVE'::character varying)::text, ('EXPIRED'::character varying)::text, ('ARCHIVED'::character varying)::text])))
);


--
-- Name: product_specifications; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.product_specifications (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    sku character varying(100) NOT NULL,
    name character varying(255) NOT NULL,
    description text,
    brand character varying(100),
    category_id uuid,
    type public.product_type NOT NULL,
    attributes jsonb,
    is_active boolean DEFAULT true,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    owner_org_id uuid,
    publish_scope text DEFAULT 'PRIVATE'::text NOT NULL,
    visibility_meta jsonb DEFAULT '{"region": "*", "channel": "*", "denied_orgs": [], "allowed_orgs": ["*"], "allowed_roles": ["*"], "visibility_scope": "PRIVATE"}'::jsonb NOT NULL,
    CONSTRAINT product_specifications_publish_scope_check CHECK ((publish_scope = ANY (ARRAY['PRIVATE'::text, 'ORG'::text, 'PUBLIC'::text, 'PARTNER'::text])))
);


--
-- Name: products_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.products_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    MAXVALUE 2147483647
    CACHE 1;


--
-- Name: provisioning_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.provisioning_events (
    id bigint NOT NULL,
    request_id uuid NOT NULL,
    event_time timestamp with time zone DEFAULT now(),
    level character varying(10) DEFAULT 'INFO'::character varying,
    message text NOT NULL,
    data jsonb
);


--
-- Name: provisioning_events_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.provisioning_events_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: provisioning_events_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.provisioning_events_id_seq OWNED BY public.provisioning_events.id;


--
-- Name: provisioning_requests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.provisioning_requests (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    action character varying(24) NOT NULL,
    order_id bigint,
    offering_id uuid NOT NULL,
    template_id uuid NOT NULL,
    resource_id uuid,
    payload jsonb NOT NULL,
    status character varying(24) DEFAULT 'QUEUED'::character varying NOT NULL,
    error_message text,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);


--
-- Name: provisioning_tasks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.provisioning_tasks (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    order_id uuid NOT NULL,
    offering_id uuid NOT NULL,
    resource_instance_id uuid,
    action character varying(30) NOT NULL,
    status character varying(30) NOT NULL,
    attempt integer DEFAULT 0,
    max_attempt integer DEFAULT 5,
    idempotency_key character varying(100),
    error_message text,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);


--
-- Name: radacct; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.radacct (
    radacctid bigint NOT NULL,
    acctsessionid text NOT NULL,
    acctuniqueid text NOT NULL,
    username text,
    realm text,
    nasipaddress inet NOT NULL,
    nasportid text,
    nasporttype text,
    acctstarttime timestamp with time zone,
    acctupdatetime timestamp with time zone,
    acctstoptime timestamp with time zone,
    acctinterval bigint,
    acctsessiontime bigint,
    acctauthentic text,
    connectinfo_start text,
    connectinfo_stop text,
    acctinputoctets bigint,
    acctoutputoctets bigint,
    calledstationid text,
    callingstationid text,
    acctterminatecause text,
    servicetype text,
    framedprotocol text,
    framedipaddress inet,
    framedipv6address inet,
    framedipv6prefix inet,
    framedinterfaceid text,
    delegatedipv6prefix inet,
    class text
);


--
-- Name: radacct_radacctid_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.radacct_radacctid_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: radacct_radacctid_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.radacct_radacctid_seq OWNED BY public.radacct.radacctid;


--
-- Name: radcheck; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.radcheck (
    id integer NOT NULL,
    username text DEFAULT ''::text NOT NULL,
    attribute text DEFAULT ''::text NOT NULL,
    op character varying(2) DEFAULT '=='::character varying NOT NULL,
    value text DEFAULT ''::text NOT NULL
);


--
-- Name: radcheck_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.radcheck_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: radcheck_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.radcheck_id_seq OWNED BY public.radcheck.id;


--
-- Name: radgroupcheck; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.radgroupcheck (
    id integer NOT NULL,
    groupname text DEFAULT ''::text NOT NULL,
    attribute text DEFAULT ''::text NOT NULL,
    op character varying(2) DEFAULT '=='::character varying NOT NULL,
    value text DEFAULT ''::text NOT NULL
);


--
-- Name: radgroupcheck_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.radgroupcheck_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: radgroupcheck_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.radgroupcheck_id_seq OWNED BY public.radgroupcheck.id;


--
-- Name: radgroupreply; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.radgroupreply (
    id integer NOT NULL,
    groupname text DEFAULT ''::text NOT NULL,
    attribute text DEFAULT ''::text NOT NULL,
    op character varying(2) DEFAULT '='::character varying NOT NULL,
    value text DEFAULT ''::text NOT NULL
);


--
-- Name: radgroupreply_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.radgroupreply_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: radgroupreply_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.radgroupreply_id_seq OWNED BY public.radgroupreply.id;


--
-- Name: radpostauth; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.radpostauth (
    id bigint NOT NULL,
    username text NOT NULL,
    pass text,
    reply text,
    calledstationid text,
    callingstationid text,
    authdate timestamp with time zone DEFAULT now() NOT NULL,
    class text
);


--
-- Name: radpostauth_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.radpostauth_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: radpostauth_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.radpostauth_id_seq OWNED BY public.radpostauth.id;


--
-- Name: radreply; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.radreply (
    id integer NOT NULL,
    username text DEFAULT ''::text NOT NULL,
    attribute text DEFAULT ''::text NOT NULL,
    op character varying(2) DEFAULT '='::character varying NOT NULL,
    value text DEFAULT ''::text NOT NULL
);


--
-- Name: radreply_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.radreply_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: radreply_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.radreply_id_seq OWNED BY public.radreply.id;


--
-- Name: radusergroup; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.radusergroup (
    id integer NOT NULL,
    username text DEFAULT ''::text NOT NULL,
    groupname text DEFAULT ''::text NOT NULL,
    priority integer DEFAULT 0 NOT NULL
);


--
-- Name: radusergroup_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.radusergroup_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: radusergroup_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.radusergroup_id_seq OWNED BY public.radusergroup.id;


--
-- Name: request_idempotency; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.request_idempotency (
    id bigint NOT NULL,
    key uuid NOT NULL,
    scope character varying(64) NOT NULL,
    fingerprint text,
    created_by character varying(128),
    status character varying(16) DEFAULT 'IN_PROGRESS'::character varying NOT NULL,
    response jsonb,
    error jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    expires_at timestamp with time zone
);


--
-- Name: request_idempotency_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.request_idempotency_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: request_idempotency_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.request_idempotency_id_seq OWNED BY public.request_idempotency.id;


--
-- Name: resource_instances; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.resource_instances (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    offering_id uuid NOT NULL,
    template_id uuid NOT NULL,
    order_id bigint,
    state character varying(24) DEFAULT 'PENDING'::character varying NOT NULL,
    attributes jsonb DEFAULT '{}'::jsonb NOT NULL,
    telemetry_bindings jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    org_id uuid NOT NULL,
    account_id uuid,
    resource_code character varying(100),
    status character varying(24) DEFAULT 'ACTIVE'::character varying,
    contract_id uuid,
    resource_kind public.resource_kind
);


--
-- Name: resource_templates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.resource_templates (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name character varying(100) NOT NULL,
    version integer DEFAULT 1,
    ui_schema jsonb,
    provision_spec jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    order_type_id integer NOT NULL,
    workflow_type character varying(100)
);


--
-- Name: role_permissions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.role_permissions (
    role_id uuid NOT NULL,
    permission_id uuid NOT NULL,
    granted_at timestamp without time zone
);


--
-- Name: roles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.roles (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    role_name character varying,
    description text,
    created_at timestamp without time zone DEFAULT now(),
    updated_at timestamp without time zone
);


--
-- Name: seq_order_details; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.seq_order_details
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: seq_order_statuses; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.seq_order_statuses
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: seq_order_subtypes; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.seq_order_subtypes
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: service_transactions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.service_transactions (
    id integer NOT NULL,
    product_id integer,
    user_id uuid,
    metadata json,
    transaction_status public.transaction_status,
    response json,
    created_at timestamp without time zone,
    updated_at timestamp without time zone
);


--
-- Name: service_transactions_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.service_transactions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    MAXVALUE 2147483647
    CACHE 1;


--
-- Name: spec_characteristics; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.spec_characteristics (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    spec_id uuid NOT NULL,
    name character varying(100) NOT NULL,
    data_type character varying(20) NOT NULL,
    unit character varying(20),
    allowed_values jsonb,
    is_required boolean DEFAULT false,
    is_configurable boolean DEFAULT false,
    scope character varying(20) DEFAULT 'commercial'::character varying,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    specification_key character varying(100) NOT NULL,
    CONSTRAINT spec_characteristics_data_type_check CHECK (((data_type)::text = ANY (ARRAY[('string'::character varying)::text, ('number'::character varying)::text, ('boolean'::character varying)::text, ('enum'::character varying)::text, ('json'::character varying)::text]))),
    CONSTRAINT spec_characteristics_scope_check CHECK (((scope)::text = ANY (ARRAY[('commercial'::character varying)::text, ('technical'::character varying)::text])))
);


--
-- Name: system_bank_accounts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.system_bank_accounts (
    old_uuid_id uuid DEFAULT gen_random_uuid() NOT NULL,
    payment_method character varying(50) NOT NULL,
    bank_name character varying(100),
    account_holder_name character varying(100) NOT NULL,
    account_identifier text NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    is_default boolean DEFAULT false NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    id integer NOT NULL,
    payment_channel character varying(50)
);


--
-- Name: system_bank_accounts_id_int_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.system_bank_accounts_id_int_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: system_bank_accounts_id_int_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.system_bank_accounts_id_int_seq OWNED BY public.system_bank_accounts.id;


--
-- Name: uploaded_files; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.uploaded_files (
    id integer NOT NULL,
    original_name character varying(255) NOT NULL,
    stored_name character varying(255) NOT NULL,
    mime_type character varying(100) NOT NULL,
    size bigint NOT NULL,
    storage_path character varying(500) NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    module character varying(255),
    CONSTRAINT uploaded_files_size_check CHECK ((size >= 0))
);


--
-- Name: uploaded_files_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.uploaded_files_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: uploaded_files_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.uploaded_files_id_seq OWNED BY public.uploaded_files.id;


--
-- Name: user_accounts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_accounts (
    user_id uuid NOT NULL,
    account_id uuid NOT NULL,
    relation_type text DEFAULT 'OWNER'::text NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    CONSTRAINT user_accounts_relation_type_check CHECK ((relation_type = ANY (ARRAY['OWNER'::text, 'CONTACT'::text, 'BILLING'::text, 'TECH'::text, 'VIEW_ONLY'::text])))
);


--
-- Name: user_bank_accounts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_bank_accounts (
    id integer NOT NULL,
    user_id uuid NOT NULL,
    bank_name character varying(100) NOT NULL,
    account_number character varying(50) NOT NULL,
    account_holder_name character varying(100) NOT NULL,
    is_default boolean DEFAULT false NOT NULL
);


--
-- Name: user_bank_accounts_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.user_bank_accounts_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: user_bank_accounts_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.user_bank_accounts_id_seq OWNED BY public.user_bank_accounts.id;


--
-- Name: user_org_members; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_org_members (
    user_id uuid NOT NULL,
    org_id uuid NOT NULL,
    org_role text NOT NULL,
    CONSTRAINT user_org_members_org_role_check CHECK ((org_role = ANY (ARRAY['OWNER'::text, 'ADMIN'::text, 'STAFF'::text, 'MITRA_OWNER'::text, 'CUSTOMER'::text, 'VIEWER'::text])))
);


--
-- Name: user_profiles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_profiles (
    id uuid NOT NULL,
    user_id uuid,
    full_name character varying,
    phone_number character varying,
    address text,
    city character varying,
    province character varying,
    postal_code character varying,
    country character varying,
    job_title character varying,
    company_name character varying,
    work_phone_number character varying,
    work_email character varying,
    facebook_url character varying,
    twitter_handle character varying,
    instagram_handle character varying,
    linkedin_url character varying,
    email_verified_at timestamp without time zone,
    phone_verified_at timestamp without time zone,
    is_two_factor_enabled boolean,
    language_preference character varying,
    theme_preference character varying,
    notification_preference jsonb,
    subscription_plan character varying,
    subscription_start_date timestamp without time zone,
    subscription_end_date timestamp without time zone,
    subscription_status character varying,
    avatar_url character varying,
    bio text,
    gender character varying(10),
    date_of_birth date,
    nationality character varying(50),
    created_at timestamp without time zone,
    updated_at timestamp without time zone
);


--
-- Name: user_roles; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_roles (
    user_id uuid NOT NULL,
    role_id uuid NOT NULL,
    assigned_at timestamp without time zone
);


--
-- Name: users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.users (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    is_active boolean,
    is_verified boolean,
    email character varying,
    username character varying,
    password character varying,
    full_name character varying,
    phone_number character varying,
    role character varying,
    last_login timestamp without time zone,
    created_at timestamp without time zone,
    updated_at timestamp without time zone,
    deleted_at timestamp without time zone,
    deposit_account_id uuid,
    deposit_balance numeric(18,2) DEFAULT 0
);


--
-- Name: ORDER_DETAIL_TYPES ORDER_DETAIL_TYPE_ID; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public."ORDER_DETAIL_TYPES" ALTER COLUMN "ORDER_DETAIL_TYPE_ID" SET DEFAULT nextval('public."ORDER_DETAIL_TYPES_ORDER_DETAIL_TYPE_ID_seq"'::regclass);


--
-- Name: account_contact_types account_contact_type_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_contact_types ALTER COLUMN account_contact_type_id SET DEFAULT nextval('public.account_contact_types_account_contact_type_id_seq'::regclass);


--
-- Name: account_contacts account_contact_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_contacts ALTER COLUMN account_contact_id SET DEFAULT nextval('public.account_contacts_account_contact_id_seq'::regclass);


--
-- Name: account_types account_type_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_types ALTER COLUMN account_type_id SET DEFAULT nextval('public.account_types_account_type_id_seq'::regclass);


--
-- Name: asset_details asset_detail_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.asset_details ALTER COLUMN asset_detail_id SET DEFAULT nextval('public.asset_details_asset_detail_id_seq'::regclass);


--
-- Name: assets asset_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.assets ALTER COLUMN asset_id SET DEFAULT nextval('public.assets_asset_id_seq'::regclass);


--
-- Name: billing_details billing_detail_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.billing_details ALTER COLUMN billing_detail_id SET DEFAULT nextval('public.billing_details_billing_detail_id_seq'::regclass);


--
-- Name: billings billing_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.billings ALTER COLUMN billing_id SET DEFAULT nextval('public.billings_billing_id_seq'::regclass);


--
-- Name: nas id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nas ALTER COLUMN id SET DEFAULT nextval('public.nas_id_seq'::regclass);


--
-- Name: order_activities order_activity_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.order_activities ALTER COLUMN order_activity_id SET DEFAULT nextval('public.order_activities_seq'::regclass);


--
-- Name: order_detail_types id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.order_detail_types ALTER COLUMN id SET DEFAULT nextval('public.order_detail_types_id_seq'::regclass);


--
-- Name: order_details order_detail_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.order_details ALTER COLUMN order_detail_id SET DEFAULT nextval('public.order_details_order_detail_id_seq'::regclass);


--
-- Name: order_statuses order_status_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.order_statuses ALTER COLUMN order_status_id SET DEFAULT nextval('public.order_statuses_order_status_id_seq'::regclass);


--
-- Name: order_subtypes order_subtype_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.order_subtypes ALTER COLUMN order_subtype_id SET DEFAULT nextval('public.order_subtypes_order_subtype_id_seq'::regclass);


--
-- Name: order_types order_type_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.order_types ALTER COLUMN order_type_id SET DEFAULT nextval('public.order_types_order_type_id_seq'::regclass);


--
-- Name: orders order_id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.orders ALTER COLUMN order_id SET DEFAULT nextval('public.orders_order_id_seq'::regclass);


--
-- Name: provisioning_events id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.provisioning_events ALTER COLUMN id SET DEFAULT nextval('public.provisioning_events_id_seq'::regclass);


--
-- Name: radacct radacctid; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.radacct ALTER COLUMN radacctid SET DEFAULT nextval('public.radacct_radacctid_seq'::regclass);


--
-- Name: radcheck id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.radcheck ALTER COLUMN id SET DEFAULT nextval('public.radcheck_id_seq'::regclass);


--
-- Name: radgroupcheck id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.radgroupcheck ALTER COLUMN id SET DEFAULT nextval('public.radgroupcheck_id_seq'::regclass);


--
-- Name: radgroupreply id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.radgroupreply ALTER COLUMN id SET DEFAULT nextval('public.radgroupreply_id_seq'::regclass);


--
-- Name: radpostauth id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.radpostauth ALTER COLUMN id SET DEFAULT nextval('public.radpostauth_id_seq'::regclass);


--
-- Name: radreply id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.radreply ALTER COLUMN id SET DEFAULT nextval('public.radreply_id_seq'::regclass);


--
-- Name: radusergroup id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.radusergroup ALTER COLUMN id SET DEFAULT nextval('public.radusergroup_id_seq'::regclass);


--
-- Name: request_idempotency id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.request_idempotency ALTER COLUMN id SET DEFAULT nextval('public.request_idempotency_id_seq'::regclass);


--
-- Name: system_bank_accounts id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.system_bank_accounts ALTER COLUMN id SET DEFAULT nextval('public.system_bank_accounts_id_int_seq'::regclass);


--
-- Name: uploaded_files id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.uploaded_files ALTER COLUMN id SET DEFAULT nextval('public.uploaded_files_id_seq'::regclass);


--
-- Name: user_bank_accounts id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_bank_accounts ALTER COLUMN id SET DEFAULT nextval('public.user_bank_accounts_id_seq'::regclass);


--
-- Name: ORDER_DETAIL_TYPES ORDER_DETAIL_TYPES_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public."ORDER_DETAIL_TYPES"
    ADD CONSTRAINT "ORDER_DETAIL_TYPES_pkey" PRIMARY KEY ("ORDER_DETAIL_TYPE_ID");


--
-- Name: users UQ_97672ac88f789774dd47f7c8be3; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT "UQ_97672ac88f789774dd47f7c8be3" UNIQUE (email);


--
-- Name: users UQ_fe0bb3f6520ee0469504521e710; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT "UQ_fe0bb3f6520ee0469504521e710" UNIQUE (username);


--
-- Name: account_contact_types account_contact_types_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_contact_types
    ADD CONSTRAINT account_contact_types_name_key UNIQUE (name);


--
-- Name: account_contact_types account_contact_types_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_contact_types
    ADD CONSTRAINT account_contact_types_pkey PRIMARY KEY (account_contact_type_id);


--
-- Name: account_contacts account_contacts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_contacts
    ADD CONSTRAINT account_contacts_pkey PRIMARY KEY (account_contact_id);


--
-- Name: account account_nik_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account
    ADD CONSTRAINT account_nik_key UNIQUE (nik);


--
-- Name: account account_passport_number_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account
    ADD CONSTRAINT account_passport_number_key UNIQUE (passport_number);


--
-- Name: account account_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account
    ADD CONSTRAINT account_pkey PRIMARY KEY (account_id);


--
-- Name: account_types account_types_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_types
    ADD CONSTRAINT account_types_name_key UNIQUE (name);


--
-- Name: account_types account_types_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_types
    ADD CONSTRAINT account_types_pkey PRIMARY KEY (account_type_id);


--
-- Name: asset_details asset_details_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.asset_details
    ADD CONSTRAINT asset_details_pk PRIMARY KEY (asset_detail_id);


--
-- Name: assets assets_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.assets
    ADD CONSTRAINT assets_pk PRIMARY KEY (asset_id);


--
-- Name: billing_details billing_details_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.billing_details
    ADD CONSTRAINT billing_details_pk PRIMARY KEY (billing_detail_id);


--
-- Name: billing_invoice_items billing_invoice_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.billing_invoice_items
    ADD CONSTRAINT billing_invoice_items_pkey PRIMARY KEY (id);


--
-- Name: billing_invoices billing_invoices_billing_no_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.billing_invoices
    ADD CONSTRAINT billing_invoices_billing_no_key UNIQUE (billing_no);


--
-- Name: billing_invoices billing_invoices_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.billing_invoices
    ADD CONSTRAINT billing_invoices_pkey PRIMARY KEY (id);


--
-- Name: billing_payment_applications billing_payment_applications_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.billing_payment_applications
    ADD CONSTRAINT billing_payment_applications_pkey PRIMARY KEY (id);


--
-- Name: billing_payments billing_payments_payment_no_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.billing_payments
    ADD CONSTRAINT billing_payments_payment_no_key UNIQUE (payment_no);


--
-- Name: billing_payments billing_payments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.billing_payments
    ADD CONSTRAINT billing_payments_pkey PRIMARY KEY (id);


--
-- Name: billings billings_pk; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.billings
    ADD CONSTRAINT billings_pk PRIMARY KEY (billing_id);


--
-- Name: brands brands_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.brands
    ADD CONSTRAINT brands_pkey PRIMARY KEY (id);


--
-- Name: bundle_components bundle_components_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bundle_components
    ADD CONSTRAINT bundle_components_pkey PRIMARY KEY (bundle_offering_id, component_offering_id);


--
-- Name: category_attribute_schemas category_attribute_schemas_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.category_attribute_schemas
    ADD CONSTRAINT category_attribute_schemas_pkey PRIMARY KEY (id);


--
-- Name: contact_requests contact_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contact_requests
    ADD CONSTRAINT contact_requests_pkey PRIMARY KEY (id);


--
-- Name: contract_lifecycle_events contract_lifecycle_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contract_lifecycle_events
    ADD CONSTRAINT contract_lifecycle_events_pkey PRIMARY KEY (id);


--
-- Name: contracts contracts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.contracts
    ADD CONSTRAINT contracts_pkey PRIMARY KEY (contract_id);


--
-- Name: deposit_accounts deposit_accounts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deposit_accounts
    ADD CONSTRAINT deposit_accounts_pkey PRIMARY KEY (id);


--
-- Name: deposit_topup_requests deposit_topup_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deposit_topup_requests
    ADD CONSTRAINT deposit_topup_requests_pkey PRIMARY KEY (id);


--
-- Name: deposit_transactions deposit_transactions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deposit_transactions
    ADD CONSTRAINT deposit_transactions_pkey PRIMARY KEY (id);


--
-- Name: deposit_withdraw_requests deposit_withdraw_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deposit_withdraw_requests
    ADD CONSTRAINT deposit_withdraw_requests_pkey PRIMARY KEY (id);


--
-- Name: digital_files digital_files_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.digital_files
    ADD CONSTRAINT digital_files_pkey PRIMARY KEY (id);


--
-- Name: gl_accounts gl_accounts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gl_accounts
    ADD CONSTRAINT gl_accounts_pkey PRIMARY KEY (id);


--
-- Name: gl_accounts gl_accounts_unique_code_org; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gl_accounts
    ADD CONSTRAINT gl_accounts_unique_code_org UNIQUE (account_code, org_id);


--
-- Name: gl_journal_entries gl_journal_entries_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gl_journal_entries
    ADD CONSTRAINT gl_journal_entries_pkey PRIMARY KEY (id);


--
-- Name: gl_journal_lines gl_journal_lines_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gl_journal_lines
    ADD CONSTRAINT gl_journal_lines_pkey PRIMARY KEY (id);


--
-- Name: landing_pages landing_pages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.landing_pages
    ADD CONSTRAINT landing_pages_pkey PRIMARY KEY (id);


--
-- Name: menus menus_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.menus
    ADD CONSTRAINT menus_pkey PRIMARY KEY (id);


--
-- Name: modules modules_module_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.modules
    ADD CONSTRAINT modules_module_key_key UNIQUE (module_key);


--
-- Name: modules modules_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.modules
    ADD CONSTRAINT modules_pkey PRIMARY KEY (id);


--
-- Name: nas nas_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nas
    ADD CONSTRAINT nas_pkey PRIMARY KEY (id);


--
-- Name: nasreload nasreload_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.nasreload
    ADD CONSTRAINT nasreload_pkey PRIMARY KEY (nasipaddress);


--
-- Name: offering_publications offering_publications_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.offering_publications
    ADD CONSTRAINT offering_publications_pkey PRIMARY KEY (offering_id, visible_to_org_id);


--
-- Name: offering_resource_templates offering_resource_templates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.offering_resource_templates
    ADD CONSTRAINT offering_resource_templates_pkey PRIMARY KEY (id);


--
-- Name: order_activities order_activities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.order_activities
    ADD CONSTRAINT order_activities_pkey PRIMARY KEY (order_activity_id);


--
-- Name: order_detail_types order_detail_types_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.order_detail_types
    ADD CONSTRAINT order_detail_types_pkey PRIMARY KEY (id);


--
-- Name: order_details order_details_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.order_details
    ADD CONSTRAINT order_details_pkey PRIMARY KEY (order_detail_id);


--
-- Name: order_status_transitions order_status_transitions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.order_status_transitions
    ADD CONSTRAINT order_status_transitions_pkey PRIMARY KEY (from_status_id, to_status_id);


--
-- Name: order_statuses order_statuses_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.order_statuses
    ADD CONSTRAINT order_statuses_pkey PRIMARY KEY (order_status_id);


--
-- Name: order_subtypes order_subtypes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.order_subtypes
    ADD CONSTRAINT order_subtypes_pkey PRIMARY KEY (order_subtype_id);


--
-- Name: order_types order_types_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.order_types
    ADD CONSTRAINT order_types_pkey PRIMARY KEY (order_type_id);


--
-- Name: orders orders_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.orders
    ADD CONSTRAINT orders_pkey PRIMARY KEY (order_id);


--
-- Name: orgs orgs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.orgs
    ADD CONSTRAINT orgs_pkey PRIMARY KEY (id);


--
-- Name: payment_applications payment_applications_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payment_applications
    ADD CONSTRAINT payment_applications_pkey PRIMARY KEY (id);


--
-- Name: payments payments_payment_no_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payments
    ADD CONSTRAINT payments_payment_no_key UNIQUE (payment_no);


--
-- Name: payments payments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payments
    ADD CONSTRAINT payments_pkey PRIMARY KEY (id);


--
-- Name: permissions permission_name_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.permissions
    ADD CONSTRAINT permission_name_unique UNIQUE (permission_name);


--
-- Name: permissions permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.permissions
    ADD CONSTRAINT permissions_pkey PRIMARY KEY (id);


--
-- Name: playing_with_neon playing_with_neon_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.playing_with_neon
    ADD CONSTRAINT playing_with_neon_pkey PRIMARY KEY (id);


--
-- Name: pricing_models pricing_models_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pricing_models
    ADD CONSTRAINT pricing_models_pkey PRIMARY KEY (id);


--
-- Name: product_attributes product_attributes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.product_attributes
    ADD CONSTRAINT product_attributes_pkey PRIMARY KEY (id);


--
-- Name: product_categories product_categories_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.product_categories
    ADD CONSTRAINT product_categories_name_key UNIQUE (name);


--
-- Name: product_categories product_categories_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.product_categories
    ADD CONSTRAINT product_categories_pkey PRIMARY KEY (id);


--
-- Name: product_configurations product_configurations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.product_configurations
    ADD CONSTRAINT product_configurations_pkey PRIMARY KEY (id);


--
-- Name: product_images product_images_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.product_images
    ADD CONSTRAINT product_images_pkey PRIMARY KEY (id);


--
-- Name: product_offering_attributes product_offering_attributes_offering_id_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.product_offering_attributes
    ADD CONSTRAINT product_offering_attributes_offering_id_name_key UNIQUE (offering_id, name);


--
-- Name: product_offering_attributes product_offering_attributes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.product_offering_attributes
    ADD CONSTRAINT product_offering_attributes_pkey PRIMARY KEY (id);


--
-- Name: product_offerings product_offerings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.product_offerings
    ADD CONSTRAINT product_offerings_pkey PRIMARY KEY (id);


--
-- Name: product_specifications product_specifications_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.product_specifications
    ADD CONSTRAINT product_specifications_pkey PRIMARY KEY (id);


--
-- Name: product_specifications product_specifications_sku_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.product_specifications
    ADD CONSTRAINT product_specifications_sku_key UNIQUE (sku);


--
-- Name: provisioning_events provisioning_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.provisioning_events
    ADD CONSTRAINT provisioning_events_pkey PRIMARY KEY (id);


--
-- Name: provisioning_requests provisioning_requests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.provisioning_requests
    ADD CONSTRAINT provisioning_requests_pkey PRIMARY KEY (id);


--
-- Name: provisioning_tasks provisioning_tasks_idempotency_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.provisioning_tasks
    ADD CONSTRAINT provisioning_tasks_idempotency_key_key UNIQUE (idempotency_key);


--
-- Name: provisioning_tasks provisioning_tasks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.provisioning_tasks
    ADD CONSTRAINT provisioning_tasks_pkey PRIMARY KEY (id);


--
-- Name: radacct radacct_acctuniqueid_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.radacct
    ADD CONSTRAINT radacct_acctuniqueid_key UNIQUE (acctuniqueid);


--
-- Name: radacct radacct_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.radacct
    ADD CONSTRAINT radacct_pkey PRIMARY KEY (radacctid);


--
-- Name: radcheck radcheck_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.radcheck
    ADD CONSTRAINT radcheck_pkey PRIMARY KEY (id);


--
-- Name: radgroupcheck radgroupcheck_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.radgroupcheck
    ADD CONSTRAINT radgroupcheck_pkey PRIMARY KEY (id);


--
-- Name: radgroupreply radgroupreply_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.radgroupreply
    ADD CONSTRAINT radgroupreply_pkey PRIMARY KEY (id);


--
-- Name: radpostauth radpostauth_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.radpostauth
    ADD CONSTRAINT radpostauth_pkey PRIMARY KEY (id);


--
-- Name: radreply radreply_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.radreply
    ADD CONSTRAINT radreply_pkey PRIMARY KEY (id);


--
-- Name: radusergroup radusergroup_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.radusergroup
    ADD CONSTRAINT radusergroup_pkey PRIMARY KEY (id);


--
-- Name: request_idempotency request_idempotency_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.request_idempotency
    ADD CONSTRAINT request_idempotency_pkey PRIMARY KEY (id);


--
-- Name: resource_instances resource_instances_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.resource_instances
    ADD CONSTRAINT resource_instances_pkey PRIMARY KEY (id);


--
-- Name: resource_templates resource_templates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.resource_templates
    ADD CONSTRAINT resource_templates_pkey PRIMARY KEY (id);


--
-- Name: roles role_name_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT role_name_unique UNIQUE (role_name);


--
-- Name: role_permissions role_permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.role_permissions
    ADD CONSTRAINT role_permissions_pkey PRIMARY KEY (role_id, permission_id);


--
-- Name: roles roles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.roles
    ADD CONSTRAINT roles_pkey PRIMARY KEY (id);


--
-- Name: service_transactions service_transactions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.service_transactions
    ADD CONSTRAINT service_transactions_pkey PRIMARY KEY (id);


--
-- Name: spec_characteristics spec_characteristics_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.spec_characteristics
    ADD CONSTRAINT spec_characteristics_pkey PRIMARY KEY (id);


--
-- Name: spec_characteristics spec_characteristics_spec_id_key_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.spec_characteristics
    ADD CONSTRAINT spec_characteristics_spec_id_key_unique UNIQUE (spec_id, specification_key);


--
-- Name: spec_characteristics spec_characteristics_spec_id_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.spec_characteristics
    ADD CONSTRAINT spec_characteristics_spec_id_name_key UNIQUE (spec_id, name);


--
-- Name: system_bank_accounts system_bank_accounts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.system_bank_accounts
    ADD CONSTRAINT system_bank_accounts_pkey PRIMARY KEY (id);


--
-- Name: uploaded_files uploaded_files_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.uploaded_files
    ADD CONSTRAINT uploaded_files_pkey PRIMARY KEY (id);


--
-- Name: uploaded_files uploaded_files_stored_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.uploaded_files
    ADD CONSTRAINT uploaded_files_stored_name_key UNIQUE (stored_name);


--
-- Name: user_accounts user_accounts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_accounts
    ADD CONSTRAINT user_accounts_pkey PRIMARY KEY (user_id, account_id);


--
-- Name: user_bank_accounts user_bank_accounts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_bank_accounts
    ADD CONSTRAINT user_bank_accounts_pkey PRIMARY KEY (id);


--
-- Name: user_org_members user_org_members_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_org_members
    ADD CONSTRAINT user_org_members_pkey PRIMARY KEY (user_id, org_id, org_role);


--
-- Name: user_profiles user_profiles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_profiles
    ADD CONSTRAINT user_profiles_pkey PRIMARY KEY (id);


--
-- Name: user_roles user_roles_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_roles
    ADD CONSTRAINT user_roles_pkey PRIMARY KEY (user_id, role_id);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: contracts_next_billing_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX contracts_next_billing_idx ON public.contracts USING btree (status, next_billing_date);


--
-- Name: contracts_org_account_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX contracts_org_account_idx ON public.contracts USING btree (org_id, account_id);


--
-- Name: gl_journal_entries_source_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX gl_journal_entries_source_idx ON public.gl_journal_entries USING btree (source_type, source_id);


--
-- Name: gl_journal_lines_journal_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX gl_journal_lines_journal_idx ON public.gl_journal_lines USING btree (journal_id);


--
-- Name: idx_attr_product; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_attr_product ON public.product_attributes USING btree (product_id);


--
-- Name: idx_bi_account; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bi_account ON public.billing_invoices USING btree (account_id);


--
-- Name: idx_bi_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bi_org ON public.billing_invoices USING btree (org_id, status);


--
-- Name: idx_bii_invoice; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bii_invoice ON public.billing_invoice_items USING btree (invoice_id);


--
-- Name: idx_bp_account; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bp_account ON public.billing_payments USING btree (account_id);


--
-- Name: idx_bp_org_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bp_org_status ON public.billing_payments USING btree (org_id, status);


--
-- Name: idx_bp_reference_no; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bp_reference_no ON public.billing_payments USING btree (reference_no);


--
-- Name: idx_bpa_invoice; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bpa_invoice ON public.billing_payment_applications USING btree (invoice_id);


--
-- Name: idx_bpa_org_account; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bpa_org_account ON public.billing_payment_applications USING btree (org_id, account_id);


--
-- Name: idx_bpa_payment; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_bpa_payment ON public.billing_payment_applications USING btree (payment_id);


--
-- Name: idx_contact_requests_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contact_requests_created_at ON public.contact_requests USING btree (created_at DESC);


--
-- Name: idx_contact_requests_email; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contact_requests_email ON public.contact_requests USING btree (email);


--
-- Name: idx_contact_requests_intent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contact_requests_intent ON public.contact_requests USING btree (intent);


--
-- Name: idx_contact_requests_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contact_requests_status ON public.contact_requests USING btree (status);


--
-- Name: idx_contract_lifecycle_contract; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contract_lifecycle_contract ON public.contract_lifecycle_events USING btree (contract_id);


--
-- Name: idx_contract_lifecycle_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_contract_lifecycle_org ON public.contract_lifecycle_events USING btree (org_id);


--
-- Name: idx_landing_pages_domain; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_landing_pages_domain ON public.landing_pages USING btree (primary_domain) WHERE (primary_domain IS NOT NULL);


--
-- Name: idx_landing_pages_owner; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_landing_pages_owner ON public.landing_pages USING btree (owner_type, owner_id);


--
-- Name: idx_ort_offering; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ort_offering ON public.offering_resource_templates USING btree (offering_id, version);


--
-- Name: idx_pa_invoice; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pa_invoice ON public.payment_applications USING btree (invoice_id);


--
-- Name: idx_pa_payment; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pa_payment ON public.payment_applications USING btree (payment_id);


--
-- Name: idx_pr_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_pr_status ON public.provisioning_requests USING btree (status);


--
-- Name: idx_resource_instances_contract_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_resource_instances_contract_id ON public.resource_instances USING btree (contract_id);


--
-- Name: idx_ua_account; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ua_account ON public.user_accounts USING btree (account_id);


--
-- Name: idx_ua_user; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ua_user ON public.user_accounts USING btree (user_id);


--
-- Name: idx_uploaded_files_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_uploaded_files_created_at ON public.uploaded_files USING btree (created_at);


--
-- Name: idx_uploaded_files_stored_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_uploaded_files_stored_name ON public.uploaded_files USING btree (stored_name);


--
-- Name: ix_offerings_owner; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_offerings_owner ON public.product_offerings USING btree (owner_org_id);


--
-- Name: ix_offerings_scope; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_offerings_scope ON public.product_offerings USING btree (publish_scope);


--
-- Name: ix_orders_buyer_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_orders_buyer_org ON public.orders USING btree (buyer_org_id);


--
-- Name: ix_orders_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_orders_org ON public.orders USING btree (org_id);


--
-- Name: ix_orders_seller_org; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_orders_seller_org ON public.orders USING btree (seller_org_id);


--
-- Name: ix_pub_effective; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_pub_effective ON public.offering_publications USING btree (effective_from, effective_to);


--
-- Name: ix_pub_visible_to; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_pub_visible_to ON public.offering_publications USING btree (visible_to_org_id);


--
-- Name: ix_specs_owner; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_specs_owner ON public.product_specifications USING btree (owner_org_id);


--
-- Name: ix_specs_scope; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_specs_scope ON public.product_specifications USING btree (publish_scope);


--
-- Name: ix_specs_visibility_meta; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX ix_specs_visibility_meta ON public.product_specifications USING gin (visibility_meta);


--
-- Name: nas_nasname; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX nas_nasname ON public.nas USING btree (nasname);


--
-- Name: order_activities_attr_gin_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX order_activities_attr_gin_idx ON public.order_activities USING gin (attr);


--
-- Name: order_activities_orderid_dtm_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX order_activities_orderid_dtm_idx ON public.order_activities USING btree (order_id, update_dtm DESC);


--
-- Name: order_details_attr_gin_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX order_details_attr_gin_idx ON public.order_details USING gin (attr);


--
-- Name: order_details_order_detail_type_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX order_details_order_detail_type_id_idx ON public.order_details USING btree (type);


--
-- Name: order_details_order_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX order_details_order_id_idx ON public.order_details USING btree (order_id);


--
-- Name: order_details_xs1_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX order_details_xs1_idx ON public.order_details USING btree (xs1);


--
-- Name: orders_attr_gin_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX orders_attr_gin_idx ON public.orders USING gin (attr);


--
-- Name: orders_code_per_tenant_uidx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX orders_code_per_tenant_uidx ON public.orders USING btree (org_id, order_code);


--
-- Name: orders_code_status_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX orders_code_status_idx ON public.orders USING btree (order_code, order_status_id);


--
-- Name: orders_open_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX orders_open_idx ON public.orders USING btree (order_status_id, create_dtm) WHERE (close_dtm IS NULL);


--
-- Name: orders_order_desc_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX orders_order_desc_idx ON public.orders USING btree (order_desc);


--
-- Name: orders_order_status_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX orders_order_status_id_idx ON public.orders USING btree (order_status_id);


--
-- Name: orders_order_subtype_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX orders_order_subtype_id_idx ON public.orders USING btree (order_subtype_id);


--
-- Name: orders_order_type_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX orders_order_type_id_idx ON public.orders USING btree (order_type_id);


--
-- Name: orders_xn1_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX orders_xn1_idx ON public.orders USING btree (xn1);


--
-- Name: orders_xn2_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX orders_xn2_idx ON public.orders USING btree (xn2);


--
-- Name: orders_xn5_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX orders_xn5_idx ON public.orders USING btree (xn5);


--
-- Name: orders_xs11_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX orders_xs11_idx ON public.orders USING btree (xs11);


--
-- Name: orders_xs2_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX orders_xs2_idx ON public.orders USING btree (xs2);


--
-- Name: orders_xs7_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX orders_xs7_idx ON public.orders USING btree (xs7);


--
-- Name: orders_xs8_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX orders_xs8_idx ON public.orders USING btree (xs8);


--
-- Name: provisioning_tasks_idempotency_key_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX provisioning_tasks_idempotency_key_idx ON public.provisioning_tasks USING btree (idempotency_key);


--
-- Name: provisioning_tasks_order_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX provisioning_tasks_order_id_idx ON public.provisioning_tasks USING btree (order_id);


--
-- Name: radacct_active_session_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX radacct_active_session_idx ON public.radacct USING btree (acctuniqueid) WHERE (acctstoptime IS NULL);


--
-- Name: radacct_bulk_close; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX radacct_bulk_close ON public.radacct USING btree (nasipaddress, acctstarttime) WHERE (acctstoptime IS NULL);


--
-- Name: radacct_calss_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX radacct_calss_idx ON public.radacct USING btree (class);


--
-- Name: radacct_start_user_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX radacct_start_user_idx ON public.radacct USING btree (acctstarttime, username);


--
-- Name: radcheck_username; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX radcheck_username ON public.radcheck USING btree (username, attribute);


--
-- Name: radgroupcheck_groupname; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX radgroupcheck_groupname ON public.radgroupcheck USING btree (groupname, attribute);


--
-- Name: radgroupreply_groupname; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX radgroupreply_groupname ON public.radgroupreply USING btree (groupname, attribute);


--
-- Name: radpostauth_class_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX radpostauth_class_idx ON public.radpostauth USING btree (class);


--
-- Name: radpostauth_username_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX radpostauth_username_idx ON public.radpostauth USING btree (username);


--
-- Name: radreply_username; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX radreply_username ON public.radreply USING btree (username, attribute);


--
-- Name: radusergroup_username; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX radusergroup_username ON public.radusergroup USING btree (username);


--
-- Name: request_idem_key_scope_uidx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX request_idem_key_scope_uidx ON public.request_idempotency USING btree (key, scope);


--
-- Name: request_idem_scope_fp_uidx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX request_idem_scope_fp_uidx ON public.request_idempotency USING btree (scope, fingerprint) WHERE (fingerprint IS NOT NULL);


--
-- Name: ux_landing_pages_owner_code; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX ux_landing_pages_owner_code ON public.landing_pages USING btree (owner_type, owner_id, code);


--
-- Name: orders trg_orders_touch; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_orders_touch BEFORE UPDATE ON public.orders FOR EACH ROW EXECUTE FUNCTION public.touch_orders_updated_at();


--
-- Name: request_idempotency trg_request_idem_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER trg_request_idem_updated_at BEFORE UPDATE ON public.request_idempotency FOR EACH ROW EXECUTE FUNCTION public.tg_set_updated_at();


--
-- Name: account account_parent_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account
    ADD CONSTRAINT account_parent_account_id_fkey FOREIGN KEY (parent_account_id) REFERENCES public.account(account_id) ON DELETE SET NULL;


--
-- Name: billing_invoice_items billing_invoice_items_invoice_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.billing_invoice_items
    ADD CONSTRAINT billing_invoice_items_invoice_id_fkey FOREIGN KEY (invoice_id) REFERENCES public.billing_invoices(id) ON DELETE CASCADE;


--
-- Name: billing_payment_applications bpa_invoice_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.billing_payment_applications
    ADD CONSTRAINT bpa_invoice_id_fkey FOREIGN KEY (invoice_id) REFERENCES public.billing_invoices(id);


--
-- Name: billing_payment_applications bpa_payment_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.billing_payment_applications
    ADD CONSTRAINT bpa_payment_id_fkey FOREIGN KEY (payment_id) REFERENCES public.billing_payments(id) ON DELETE CASCADE;


--
-- Name: bundle_components bundle_components_bundle_offering_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bundle_components
    ADD CONSTRAINT bundle_components_bundle_offering_id_fkey FOREIGN KEY (bundle_offering_id) REFERENCES public.product_offerings(id) ON DELETE CASCADE;


--
-- Name: bundle_components bundle_components_component_offering_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.bundle_components
    ADD CONSTRAINT bundle_components_component_offering_id_fkey FOREIGN KEY (component_offering_id) REFERENCES public.product_offerings(id) ON DELETE CASCADE;


--
-- Name: category_attribute_schemas category_attribute_schemas_category_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.category_attribute_schemas
    ADD CONSTRAINT category_attribute_schemas_category_id_fkey FOREIGN KEY (category_id) REFERENCES public.product_categories(id) ON DELETE CASCADE;


--
-- Name: deposit_topup_requests deposit_topup_requests_deposit_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deposit_topup_requests
    ADD CONSTRAINT deposit_topup_requests_deposit_account_id_fkey FOREIGN KEY (deposit_account_id) REFERENCES public.deposit_accounts(id) ON DELETE CASCADE;


--
-- Name: deposit_transactions deposit_transactions_deposit_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deposit_transactions
    ADD CONSTRAINT deposit_transactions_deposit_account_id_fkey FOREIGN KEY (deposit_account_id) REFERENCES public.deposit_accounts(id) ON DELETE CASCADE;


--
-- Name: deposit_withdraw_requests deposit_withdraw_requests_deposit_account_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deposit_withdraw_requests
    ADD CONSTRAINT deposit_withdraw_requests_deposit_account_id_fkey FOREIGN KEY (deposit_account_id) REFERENCES public.deposit_accounts(id) ON DELETE CASCADE;


--
-- Name: account fk_account_account_type; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account
    ADD CONSTRAINT fk_account_account_type FOREIGN KEY (account_type_id) REFERENCES public.account_types(account_type_id) ON DELETE RESTRICT;


--
-- Name: account_contacts fk_account_contact_account; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_contacts
    ADD CONSTRAINT fk_account_contact_account FOREIGN KEY (account_id) REFERENCES public.account(account_id) ON DELETE CASCADE;


--
-- Name: account_contacts fk_account_contact_type; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.account_contacts
    ADD CONSTRAINT fk_account_contact_type FOREIGN KEY (account_contact_type_id) REFERENCES public.account_contact_types(account_contact_type_id) ON DELETE RESTRICT;


--
-- Name: menus fk_menu_module; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.menus
    ADD CONSTRAINT fk_menu_module FOREIGN KEY (module_id) REFERENCES public.modules(id) ON DELETE CASCADE;


--
-- Name: role_permissions fk_permission_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.role_permissions
    ADD CONSTRAINT fk_permission_id FOREIGN KEY (permission_id) REFERENCES public.permissions(id);


--
-- Name: permissions fk_permission_module; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.permissions
    ADD CONSTRAINT fk_permission_module FOREIGN KEY (module_id) REFERENCES public.modules(id) ON DELETE CASCADE;


--
-- Name: product_specifications fk_product_category; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.product_specifications
    ADD CONSTRAINT fk_product_category FOREIGN KEY (category_id) REFERENCES public.product_categories(id) ON DELETE SET NULL;


--
-- Name: resource_instances fk_resource_instances_contract; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.resource_instances
    ADD CONSTRAINT fk_resource_instances_contract FOREIGN KEY (contract_id) REFERENCES public.contracts(contract_id) ON DELETE RESTRICT;


--
-- Name: role_permissions fk_role_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.role_permissions
    ADD CONSTRAINT fk_role_id FOREIGN KEY (role_id) REFERENCES public.roles(id);


--
-- Name: user_roles fk_role_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_roles
    ADD CONSTRAINT fk_role_id FOREIGN KEY (role_id) REFERENCES public.roles(id);


--
-- Name: user_accounts fk_ua_account; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_accounts
    ADD CONSTRAINT fk_ua_account FOREIGN KEY (account_id) REFERENCES public.account(account_id) ON DELETE CASCADE;


--
-- Name: user_accounts fk_ua_user; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_accounts
    ADD CONSTRAINT fk_ua_user FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_bank_accounts fk_user; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_bank_accounts
    ADD CONSTRAINT fk_user FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: user_profiles fk_user_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_profiles
    ADD CONSTRAINT fk_user_id FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: user_roles fk_user_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_roles
    ADD CONSTRAINT fk_user_id FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: users fk_users_deposit_account; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT fk_users_deposit_account FOREIGN KEY (deposit_account_id) REFERENCES public.deposit_accounts(id) ON DELETE SET NULL;


--
-- Name: gl_journal_lines gl_journal_lines_journal_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.gl_journal_lines
    ADD CONSTRAINT gl_journal_lines_journal_id_fkey FOREIGN KEY (journal_id) REFERENCES public.gl_journal_entries(id) ON DELETE CASCADE;


--
-- Name: offering_publications offering_publications_offering_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.offering_publications
    ADD CONSTRAINT offering_publications_offering_id_fkey FOREIGN KEY (offering_id) REFERENCES public.product_offerings(id) ON DELETE CASCADE;


--
-- Name: order_details order_details_type_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.order_details
    ADD CONSTRAINT order_details_type_fk FOREIGN KEY (type) REFERENCES public.order_detail_types(id) ON UPDATE CASCADE;


--
-- Name: order_status_transitions order_status_transitions_from_status_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.order_status_transitions
    ADD CONSTRAINT order_status_transitions_from_status_id_fkey FOREIGN KEY (from_status_id) REFERENCES public.order_statuses(order_status_id) ON UPDATE CASCADE;


--
-- Name: order_status_transitions order_status_transitions_to_status_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.order_status_transitions
    ADD CONSTRAINT order_status_transitions_to_status_id_fkey FOREIGN KEY (to_status_id) REFERENCES public.order_statuses(order_status_id) ON UPDATE CASCADE;


--
-- Name: order_subtypes order_subtypes_order_type_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.order_subtypes
    ADD CONSTRAINT order_subtypes_order_type_id_fkey FOREIGN KEY (order_type_id) REFERENCES public.order_types(order_type_id);


--
-- Name: orders orders_order_status_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.orders
    ADD CONSTRAINT orders_order_status_fk FOREIGN KEY (order_status_id) REFERENCES public.order_statuses(order_status_id) ON UPDATE CASCADE;


--
-- Name: orders orders_order_type_fk; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.orders
    ADD CONSTRAINT orders_order_type_fk FOREIGN KEY (order_type_id) REFERENCES public.order_types(order_type_id) ON UPDATE CASCADE;


--
-- Name: orgs orgs_parent_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.orgs
    ADD CONSTRAINT orgs_parent_org_id_fkey FOREIGN KEY (parent_org_id) REFERENCES public.orgs(id);


--
-- Name: payment_applications payment_applications_invoice_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payment_applications
    ADD CONSTRAINT payment_applications_invoice_id_fkey FOREIGN KEY (invoice_id) REFERENCES public.billing_invoices(id) ON DELETE CASCADE;


--
-- Name: payment_applications payment_applications_payment_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.payment_applications
    ADD CONSTRAINT payment_applications_payment_id_fkey FOREIGN KEY (payment_id) REFERENCES public.payments(id) ON DELETE CASCADE;


--
-- Name: pricing_models pricing_models_offering_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.pricing_models
    ADD CONSTRAINT pricing_models_offering_id_fkey FOREIGN KEY (offering_id) REFERENCES public.product_offerings(id) ON DELETE CASCADE;


--
-- Name: product_offering_attributes product_offering_attributes_offering_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.product_offering_attributes
    ADD CONSTRAINT product_offering_attributes_offering_id_fkey FOREIGN KEY (offering_id) REFERENCES public.product_offerings(id) ON DELETE CASCADE;


--
-- Name: product_offering_attributes product_offering_attributes_spec_characteristic_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.product_offering_attributes
    ADD CONSTRAINT product_offering_attributes_spec_characteristic_id_fkey FOREIGN KEY (spec_characteristic_id) REFERENCES public.spec_characteristics(id);


--
-- Name: product_offerings product_offerings_spec_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.product_offerings
    ADD CONSTRAINT product_offerings_spec_id_fkey FOREIGN KEY (spec_id) REFERENCES public.product_specifications(id) ON DELETE CASCADE;


--
-- Name: product_specifications product_specifications_owner_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.product_specifications
    ADD CONSTRAINT product_specifications_owner_org_id_fkey FOREIGN KEY (owner_org_id) REFERENCES public.orgs(id) ON DELETE SET NULL;


--
-- Name: service_transactions service_transactions_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.service_transactions
    ADD CONSTRAINT service_transactions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id);


--
-- Name: spec_characteristics spec_characteristics_spec_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.spec_characteristics
    ADD CONSTRAINT spec_characteristics_spec_id_fkey FOREIGN KEY (spec_id) REFERENCES public.product_specifications(id) ON DELETE CASCADE;


--
-- Name: user_org_members user_org_members_org_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_org_members
    ADD CONSTRAINT user_org_members_org_id_fkey FOREIGN KEY (org_id) REFERENCES public.orgs(id);


--
-- PostgreSQL database dump complete
--

