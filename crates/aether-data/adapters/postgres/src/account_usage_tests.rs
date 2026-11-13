use super::*;
use std::str::FromStr;

#[tokio::test]
#[ignore = "requires a disposable loopback afssr_feedback_test database"]
async fn personal_usage_ledger_isolation_expiry_models_and_boundaries() {
    let options = sqlx::postgres::PgConnectOptions::from_str(
        &std::env::var("AFSSR_ACCOUNT_USAGE_TEST_DATABASE_URL").expect("explicit test DB required"),
    )
    .unwrap();
    assert!(matches!(options.get_host(), "localhost" | "127.0.0.1"));
    assert_eq!(options.get_database(), Some("afssr_feedback_test"));
    let setup = PgPool::connect_with(options.clone()).await.unwrap();
    let schema = format!("account_usage_test_{}", uuid::Uuid::new_v4().simple());
    sqlx::query(&format!("CREATE SCHEMA {schema}"))
        .execute(&setup)
        .await
        .unwrap();
    let path = schema.clone();
    let pool = sqlx::postgres::PgPoolOptions::new()
        .after_connect(move |connection, _| {
            let path = path.clone();
            Box::pin(async move {
                sqlx::query(&format!("SET search_path TO {path}"))
                    .execute(connection)
                    .await?;
                Ok(())
            })
        })
        .connect_with(options)
        .await
        .unwrap();
    sqlx::raw_sql("CREATE TABLE usage_cost_reservations(subject_id text, model text, admitted_at timestamptz, state text, actual_cost_units bigint, reserved_cost_units bigint, reservation_expires_at timestamptz);
      INSERT INTO usage_cost_reservations SELECT u,m,to_timestamp(t),s,a,r,to_timestamp(e) FROM (VALUES
      ('self','fabel',100,'finalized',125,999,0),
      ('self','other',101,'finalized',75,999,0),
      ('self','fabel',102,'reserved',999,20,301),
      ('self','other',103,'reserved',999,10,301),
      ('self','fabel',104,'reserved',999,600,300),
      ('self','fabel',105,'released',999,600,301),
      ('other-user','fabel',106,'finalized',999,600,301),
      ('self','fabel',99,'finalized',999,600,301),
      ('self','fabel',200,'finalized',999,600,301)
      ) x(u,m,t,s,a,r,e);").execute(&pool).await.unwrap();
    let repo = PostgresAccountUsageRepository::new(pool.clone());
    let windows = vec![
        AccountUsageWindow {
            models: None,
            model: None,
            from: 100,
            until: 200,
        },
        AccountUsageWindow {
            models: None,
            model: Some("fabel".into()),
            from: 100,
            until: 200,
        },
        AccountUsageWindow {
            models: None,
            model: Some("absent".into()),
            from: 100,
            until: 200,
        },
        AccountUsageWindow {
            models: Some(vec!["fabel".into(), "other".into()]),
            model: None,
            from: 100,
            until: 200,
        },
    ];
    let amounts = repo.read_windows("self", 300, &windows).await.unwrap();
    assert_eq!(
        (amounts[0].finalized_units, amounts[0].reserved_units),
        (200, 30)
    );
    assert_eq!(
        (amounts[1].finalized_units, amounts[1].reserved_units),
        (125, 20)
    );
    assert_eq!(
        (amounts[2].finalized_units, amounts[2].reserved_units),
        (0, 0)
    );
    assert_eq!(
        (amounts[3].finalized_units, amounts[3].reserved_units),
        (200, 30)
    );
    assert_eq!(
        repo.read_windows("other-user", 300, &windows)
            .await
            .unwrap()[0]
            .finalized_units,
        999
    );
    assert_eq!(
        repo.read_windows("self", 301, &windows).await.unwrap()[0].reserved_units,
        0
    );
    assert!(repo.read_windows("self", 300, &[]).await.is_err());
    assert!(repo
        .read_windows("self", 300, &vec![windows[0].clone(); 129])
        .await
        .is_err());
    // Exercise the production row lock and selectors against this already isolated schema.
    use aether_data_contracts::repository::settlement::*;
    sqlx::raw_sql(
        "CREATE TABLE users(id text PRIMARY KEY); INSERT INTO users VALUES ('family-subject'),('zero-new-user');
        ALTER TABLE usage_cost_reservations ADD COLUMN request_id text,
          ADD COLUMN reservation_token text DEFAULT md5(random()::text) PRIMARY KEY,
          ADD COLUMN retain_until timestamptz, ADD COLUMN finalized_at timestamptz,
          ADD COLUMN created_at timestamptz, ADD COLUMN updated_at timestamptz;",
    )
    .execute(&pool)
    .await
    .unwrap();
    let settlement = crate::settlement::SqlxSettlementRepository::new(pool.clone());
    let make_input = |index: usize, model: &str, amount: u64, family: bool| {
        let total = UsagePolicyCostWindow {
            rolling_seconds: None,
            model: None,
            models: None,
            window_id: "total".into(),
            starts_at_unix_secs: 100,
            ends_at_unix_secs: 200,
            limit_cost_units: 100,
        };
        let mut checked = vec![total.clone()];
        if family {
            checked.push(UsagePolicyCostWindow {
                rolling_seconds: None,
                models: Some(vec!["fabel".into(), "other".into()]),
                window_id: "family:shared".into(),
                limit_cost_units: 30,
                ..total
            });
        }
        ReserveUsagePolicyCostInput {
            model: Some(model.into()),
            request_id: format!("family-{index}"),
            subject_id: "family-subject".into(),
            reservation_token: format!("family-{index}"),
            admitted_at_unix_secs: 150,
            reserved_cost_units: amount,
            reservation_expires_at_unix_secs: 1000,
            retain_until_unix_secs: 2000,
            windows: checked,
        }
    };
    let mut zero = make_input(1000, "fabel", 1, false);
    zero.subject_id = "zero-new-user".into();
    zero.windows[0].limit_cost_units = 0;
    assert!(matches!(
        settlement.reserve_usage_policy_cost(zero).await.unwrap(),
        ReserveUsagePolicyCostOutcome::Rejected {
            limit_cost_units: 0,
            used_cost_units: 0,
            retry_after_secs: None,
            ..
        }
    ));
    assert_eq!(
        repo.read_windows("zero-new-user", 150, &windows)
            .await
            .unwrap()[0]
            .reserved_units,
        0
    );
    let mut tasks = Vec::new();
    for index in 0..16 {
        let settlement = settlement.clone();
        let input = make_input(
            index,
            if index % 2 == 0 { "fabel" } else { "other" },
            10,
            true,
        );
        tasks.push(tokio::spawn(async move {
            settlement.reserve_usage_policy_cost(input).await.unwrap()
        }));
    }
    let mut allowed = 0;
    for task in tasks {
        allowed += usize::from(matches!(
            task.await.unwrap(),
            ReserveUsagePolicyCostOutcome::Allowed { .. }
        ));
    }
    assert_eq!(
        allowed, 3,
        "two member aliases must share the same atomic 30-unit cap"
    );
    assert!(matches!(
        settlement
            .reserve_usage_policy_cost(make_input(100, "unrelated", 70, false))
            .await
            .unwrap(),
        ReserveUsagePolicyCostOutcome::Allowed { .. }
    ));
    assert!(matches!(
        settlement
            .reserve_usage_policy_cost(make_input(101, "unrelated", 1, false))
            .await
            .unwrap(),
        ReserveUsagePolicyCostOutcome::Rejected {
            used_cost_units: 100,
            retry_after_secs: None,
            ..
        }
    ));
    let amounts = repo
        .read_windows("family-subject", 300, &windows)
        .await
        .unwrap();
    assert_eq!(amounts[0].reserved_units, 100);
    assert_eq!(amounts[3].reserved_units, 30);
    let rows: i64 = sqlx::query_scalar(
        "SELECT COUNT(*) FROM usage_cost_reservations WHERE subject_id='family-subject'",
    )
    .fetch_one(&pool)
    .await
    .unwrap();
    assert_eq!(
        rows, 4,
        "one durable charge per admitted request, not per pool"
    );
    let mut rolling = make_input(102, "unrelated", 1, false);
    rolling.windows[0].rolling_seconds = Some(50);
    assert!(matches!(
        settlement
            .reserve_usage_policy_cost(rolling.clone())
            .await
            .unwrap(),
        ReserveUsagePolicyCostOutcome::Rejected {
            used_cost_units: 100,
            retry_after_secs: Some(51),
            ..
        }
    ));
    // A pending reservation can expire before it slides out of the rolling window.
    sqlx::query("UPDATE usage_cost_reservations SET reservation_expires_at=to_timestamp(155) WHERE subject_id='family-subject' AND model='unrelated'")
        .execute(&pool).await.unwrap();
    assert!(matches!(
        settlement.reserve_usage_policy_cost(rolling).await.unwrap(),
        ReserveUsagePolicyCostOutcome::Rejected {
            used_cost_units: 100,
            retry_after_secs: Some(5),
            ..
        }
    ));
    pool.close().await;
    sqlx::query(&format!("DROP SCHEMA {schema} CASCADE"))
        .execute(&setup)
        .await
        .unwrap();
    setup.close().await;
}
