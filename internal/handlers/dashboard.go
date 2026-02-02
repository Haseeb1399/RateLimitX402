package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// DashboardHandler returns an HTTP handler that serves a live CPU utilization graph.
func DashboardHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(dashboardHTML))
	}
}

// GinDashboardHandler returns a Gin handler that serves a live CPU utilization graph.
func GinDashboardHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(dashboardHTML))
	}
}

const dashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>CPU Monitor</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body {
            font-family: 'Segoe UI', system-ui, sans-serif;
            background: linear-gradient(135deg, #fef6f9 0%, #f0f4ff 50%, #e8f5f0 100%);
            min-height: 100vh;
            display: flex;
            flex-direction: column;
            align-items: center;
            padding: 2rem;
            color: #5a5a7a;
        }
        h1 {
            font-size: 2rem;
            margin-bottom: 0.5rem;
            background: linear-gradient(90deg, #b8a9c9, #f4b8c5);
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
        }
        .subtitle {
            color: #9a9ab8;
            margin-bottom: 2rem;
            font-size: 0.9rem;
        }
        .card {
            background: rgba(255, 255, 255, 0.7);
            backdrop-filter: blur(10px);
            border-radius: 20px;
            border: 1px solid rgba(184, 169, 201, 0.3);
            padding: 2rem;
            width: 100%;
            max-width: 900px;
            box-shadow: 0 8px 32px rgba(184, 169, 201, 0.2);
        }
        .stats {
            display: flex;
            gap: 2rem;
            margin-bottom: 1.5rem;
        }
        .stat-box {
            background: linear-gradient(135deg, #fce4ec 0%, #e8f5e9 100%);
            padding: 1rem 1.5rem;
            border-radius: 14px;
            border: 1px solid rgba(244, 184, 197, 0.3);
        }
        .stat-label { font-size: 0.75rem; color: #7a7a9a; text-transform: uppercase; letter-spacing: 0.5px; font-weight: 500; }
        .stat-value { font-size: 1.5rem; font-weight: 700; color: #5c4a72; }
        .stat-box-error { background: linear-gradient(135deg, #fce4e4 0%, #ffe8e0 100%); border-color: rgba(231, 76, 60, 0.2); }
        .stat-box-error .stat-value { color: #c0392b; }
        .chart-container { position: relative; height: 350px; }
    </style>
</head>
<body>
    <h1>🌸 CPU Monitor</h1>
    <p class="subtitle">Real-time CPU utilization from /cpu endpoint</p>
    
    <div class="card">
        <div class="stats">
            <div class="stat-box">
                <div class="stat-label">Current</div>
                <div class="stat-value" id="current">--</div>
            </div>
            <div class="stat-box">
                <div class="stat-label">Average</div>
                <div class="stat-value" id="average">--</div>
            </div>
            <div class="stat-box">
                <div class="stat-label">Peak</div>
                <div class="stat-value" id="peak">--</div>
            </div>
            <div class="stat-box stat-box-error">
                <div class="stat-label">Errors</div>
                <div class="stat-value" id="errors">0</div>
            </div>
        </div>
        <div class="chart-container">
            <canvas id="cpuChart"></canvas>
        </div>
    </div>

    <script>
        const MAX_POINTS = 300;
        const labels = [];
        const data = [];
        const errorData = [];
        const errorCodes = [];
        let peak = 0;
        let errorCount = 0;

        const ctx = document.getElementById('cpuChart').getContext('2d');
        const chart = new Chart(ctx, {
            type: 'line',
            data: {
                labels: labels,
                datasets: [
                    {
                        label: 'CPU %',
                        data: data,
                        borderColor: '#7a5a8c',
                        backgroundColor: 'rgba(122, 90, 140, 0.15)',
                        borderWidth: 2,
                        fill: true,
                        tension: 0.4,
                        pointRadius: 0,
                        spanGaps: false
                    },
                    {
                        label: 'Errors',
                        data: errorData,
                        borderColor: '#c0392b',
                        backgroundColor: '#e74c3c',
                        pointRadius: 6,
                        pointHoverRadius: 8,
                        pointStyle: 'circle',
                        pointBorderWidth: 3,
                        showLine: false,
                        fill: false
                    }
                ]
            },
            options: {
                responsive: true,
                maintainAspectRatio: false,
                animation: { duration: 300 },
                scales: {
                    y: {
                        min: 0,
                        max: 100,
                        grid: { color: 'rgba(184, 169, 201, 0.2)' },
                        ticks: { color: '#9a9ab8', callback: v => v + '%' }
                    },
                    x: {
                        grid: { display: false },
                        ticks: { color: '#9a9ab8', maxTicksLimit: 10 }
                    }
                },
                plugins: {
                    legend: { display: false },
                    tooltip: {
                        callbacks: {
                            label: function(context) {
                                if (context.datasetIndex === 1 && context.raw !== null) {
                                    const code = errorCodes[context.dataIndex];
                                    return 'Error: HTTP ' + code;
                                }
                                return context.dataset.label + ': ' + context.raw + '%';
                            }
                        }
                    }
                }
            }
        });

        async function fetchCPU() {
            const time = new Date().toLocaleTimeString();
            
            try {
                const res = await fetch('/cpu');
                
                if (data.length >= MAX_POINTS) {
                    labels.shift();
                    data.shift();
                    errorData.shift();
                    errorCodes.shift();
                }
                
                if (!res.ok) {
                    // Non-200 response - show error marker at 50% height
                    errorCount++;
                    labels.push(time);
                    data.push(null);
                    errorData.push(50);
                    errorCodes.push(res.status);
                    document.getElementById('current').textContent = res.status;
                    document.getElementById('errors').textContent = errorCount;
                    chart.update();
                    return;
                }
                
                const json = await res.json();
                const util = json.utilization.toFixed(1);
                const displayTime = new Date(json.timestamp).toLocaleTimeString();

                labels.push(displayTime);
                data.push(parseFloat(util));
                errorData.push(null);
                errorCodes.push(null);

                if (parseFloat(util) > peak) peak = parseFloat(util);
                const validData = data.filter(d => d !== null);
                const avg = validData.length ? (validData.reduce((a, b) => a + b, 0) / validData.length).toFixed(1) : '0.0';

                document.getElementById('current').textContent = util + '%';
                document.getElementById('average').textContent = avg + '%';
                document.getElementById('peak').textContent = peak.toFixed(1) + '%';

                chart.update();
            } catch (e) {
                errorCount++;
                if (data.length >= MAX_POINTS) {
                    labels.shift();
                    data.shift();
                    errorData.shift();
                }
                labels.push(time);
                data.push(null);
                errorData.push(50);
                document.getElementById('current').textContent = 'ERR';
                document.getElementById('errors').textContent = errorCount;
                chart.update();
                console.error('Failed to fetch CPU:', e);
            }
        }

        fetchCPU();
        setInterval(fetchCPU, 200);
    </script>
</body>
</html>`
