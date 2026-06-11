import React, { useEffect, useState } from 'react';
import { Card, Spin } from '@douyinfe/semi-ui';
import OpenAppList from '../../pages/Setting/OpenApp/OpenAppList';
import { API } from '../../helpers';

const OpenAppSetting = () => {
  const [loading, setLoading] = useState(false);
  const [apps, setApps] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(0);
  const pageSize = 10;

  const fetchApps = async (p = 0) => {
    setLoading(true);
    try {
      const res = await API.get(`/api/openapp/?p=${p}&page_size=${pageSize}`);
      const { success, data, total } = res.data;
      if (success) {
        setApps(data || []);
        setTotal(total || 0);
      }
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchApps(page);
  }, [page]);

  return (
    <Card>
      <Spin spinning={loading}>
        <OpenAppList
          apps={apps}
          total={total}
          page={page}
          pageSize={pageSize}
          onPageChange={setPage}
          onRefresh={() => fetchApps(page)}
        />
      </Spin>
    </Card>
  );
};

export default OpenAppSetting;
